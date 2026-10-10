package billing

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cached Asia/Shanghai timezone to avoid repeated disk I/O on every request
var shanghaiLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}()

// ValidateSlotsOverlap ensures discrete time windows do not overlap on the same day of the week.
func ValidateSlotsOverlap(slotsJSON string) error {
	if strings.TrimSpace(slotsJSON) == "" {
		return nil
	}
	var slots []OffPeakSlot
	if err := json.Unmarshal([]byte(slotsJSON), &slots); err != nil {
		return fmt.Errorf("优惠时段格式无效: %w", err)
	}
	if len(slots) <= 1 {
		return nil
	}

	type interval struct {
		start int
		end   int
	}

	getIntervals := func(sStr, eStr string) ([]interval, error) {
		sM := parseTimeToMinutes(sStr)
		eM := parseTimeToMinutes(eStr)
		if sM < 0 || sM >= 1440 || eM < 0 || eM >= 1440 {
			return nil, fmt.Errorf("时间格式不正确 (需为 HH:MM)")
		}
		if sM == eM {
			return nil, fmt.Errorf("开始时间与结束时间不能相同 (%s)", sStr)
		}
		if sM < eM {
			return []interval{{start: sM, end: eM}}, nil
		}
		// Cross midnight
		return []interval{{start: sM, end: 1440}, {start: 0, end: eM}}, nil
	}

	dayNames := map[int]string{1: "周一", 2: "周二", 3: "周三", 4: "周四", 5: "周五", 6: "周六", 7: "周日"}

	for i := 0; i < len(slots); i++ {
		s1 := slots[i]
		int1, err := getIntervals(s1.Start, s1.End)
		if err != nil {
			name := s1.Name
			if name == "" {
				name = fmt.Sprintf("时段 %d", i+1)
			}
			return fmt.Errorf("[%s] %w", name, err)
		}
		days1 := s1.Days
		if len(days1) == 0 {
			days1 = []int{1, 2, 3, 4, 5, 6, 7}
		}

		for j := i + 1; j < len(slots); j++ {
			s2 := slots[j]
			int2, err := getIntervals(s2.Start, s2.End)
			if err != nil {
				name := s2.Name
				if name == "" {
					name = fmt.Sprintf("时段 %d", j+1)
				}
				return fmt.Errorf("[%s] %w", name, err)
			}
			days2 := s2.Days
			if len(days2) == 0 {
				days2 = []int{1, 2, 3, 4, 5, 6, 7}
			}

			// Check common days
			var commonDays []int
			for _, d1 := range days1 {
				for _, d2 := range days2 {
					if d1 == d2 {
						commonDays = append(commonDays, d1)
						break
					}
				}
			}
			if len(commonDays) == 0 {
				continue
			}

			// Check interval overlap
			overlap := false
			for _, r1 := range int1 {
				for _, r2 := range int2 {
					maxS := r1.start
					if r2.start > maxS {
						maxS = r2.start
					}
					minE := r1.end
					if r2.end < minE {
						minE = r2.end
					}
					if maxS < minE {
						overlap = true
						break
					}
				}
				if overlap {
					break
				}
			}

			if overlap {
				var dayStrs []string
				for _, d := range commonDays {
					dayStrs = append(dayStrs, dayNames[d])
				}
				name1 := s1.Name
				if name1 == "" {
					name1 = fmt.Sprintf("时段 %d", i+1)
				}
				name2 := s2.Name
				if name2 == "" {
					name2 = fmt.Sprintf("时段 %d", j+1)
				}
				return fmt.Errorf("[%s] (%s-%s) 与 [%s] (%s-%s) 在 %s 存在时间重叠，请调整时段",
					name1, s1.Start, s1.End, name2, s2.Start, s2.End, strings.Join(dayStrs, "、"))
			}
		}
	}

	return nil
}

// IsOffPeakDefault evaluates whether current server time matches DeepSeek official off-peak hours.
func IsOffPeakDefault(t time.Time) bool {
	localT := t.In(shanghaiLoc)
	weekday := localT.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return true
	}
	m := localT.Hour()*60 + localT.Minute()
	if (m >= 540 && m < 720) || (m >= 840 && m < 1080) {
		return false
	}
	return true
}

func parseTimeToMinutes(s string) int {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	return h*60 + m
}
