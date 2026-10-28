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

// offPeakCustomMode is the only mode whose strategy reads OffPeakSlots.
const offPeakCustomMode = "custom"

// ValidateOffPeakSlots validates the off-peak slot configuration on a write path.
//
// Slots are evaluated only by CustomOffPeakStrategy, i.e. when OffPeakMode is
// "custom" (see offPeakStrategies); for every other mode the engine ignores them.
// Validation follows execution: in a non-custom mode the slots are skipped
// entirely, because rejecting configuration that can never influence a price
// would refuse a save for no reason. The fields the engine does read in those
// modes (start/end/discount) are validated by ValidatePriceRecord.
//
// This is the only exported slot-validation entry point. There is deliberately no
// mode-less wrapper (a former ValidateSlotsOverlap helper): it let a caller
// validate slots the engine would never read.
func ValidateOffPeakSlots(mode, slotsJSON string) error {
	if strings.ToLower(strings.TrimSpace(mode)) != offPeakCustomMode {
		return nil
	}
	return validateSlots(slotsJSON)
}

// validateSlots performs the mode-independent slot validation: the fields the
// engine actually reads (discount, days, time window) and the same-day overlap
// check.
func validateSlots(slotsJSON string) error {
	if strings.TrimSpace(slotsJSON) == "" {
		return nil
	}
	var slots []OffPeakSlot
	if err := json.Unmarshal([]byte(slotsJSON), &slots); err != nil {
		return fmt.Errorf("优惠时段格式无效: %w", err)
	}
	for i := range slots {
		if err := validateOffPeakSlot(slots[i], i); err != nil {
			return err
		}
	}
	if len(slots) <= 1 {
		return nil
	}

	type interval struct {
		start int
		end   int
	}

	// Times are already known to be valid and non-equal, so this only has to turn
	// a slot into one or two half-open minute ranges.
	getIntervals := func(slot OffPeakSlot) []interval {
		sM := parseTimeToMinutes(slot.Start)
		eM := parseTimeToMinutes(slot.End)
		if sM < eM {
			return []interval{{start: sM, end: eM}}
		}
		// Cross midnight
		return []interval{{start: sM, end: 1440}, {start: 0, end: eM}}
	}

	dayNames := map[int]string{1: "周一", 2: "周二", 3: "周三", 4: "周四", 5: "周五", 6: "周六", 7: "周日"}

	for i := 0; i < len(slots); i++ {
		s1 := slots[i]
		int1 := getIntervals(s1)
		days1 := s1.Days
		if len(days1) == 0 {
			days1 = []int{1, 2, 3, 4, 5, 6, 7}
		}

		for j := i + 1; j < len(slots); j++ {
			s2 := slots[j]
			int2 := getIntervals(s2)
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
				return fmt.Errorf("[%s] (%s-%s) 与 [%s] (%s-%s) 在 %s 存在时间重叠，请调整时段",
					offPeakSlotName(s1, i), s1.Start, s1.End,
					offPeakSlotName(s2, j), s2.Start, s2.End, strings.Join(dayStrs, "、"))
			}
		}
	}

	return nil
}

// validateOffPeakSlot rejects slot values the engine cannot honour. An
// out-of-range day made the slot never match (silently no discount) and an
// out-of-range discount used to fall back to the row discount, so both were
// accepted on write and then priced differently than configured.
func validateOffPeakSlot(slot OffPeakSlot, index int) error {
	name := offPeakSlotName(slot, index)
	if slot.Discount <= 0 || slot.Discount >= 1.0 {
		return fmt.Errorf("[%s] 折扣必须大于 0 且小于 1（当前 %g）", name, slot.Discount)
	}
	for _, day := range slot.Days {
		if day < 1 || day > 7 {
			return fmt.Errorf("[%s] 生效日必须是 1-7（1=周一 … 7=周日），当前包含 %d", name, day)
		}
	}
	startM := parseTimeToMinutes(slot.Start)
	endM := parseTimeToMinutes(slot.End)
	if startM < 0 || startM >= 1440 || endM < 0 || endM >= 1440 {
		return fmt.Errorf("[%s] 时间格式不正确 (需为 HH:MM)", name)
	}
	if startM == endM {
		return fmt.Errorf("[%s] 开始时间与结束时间不能相同 (%s)", name, slot.Start)
	}
	return nil
}

// offPeakSlotName renders a slot for error messages: its label when it has one,
// otherwise its position in the list.
func offPeakSlotName(slot OffPeakSlot, index int) string {
	if slot.Name != "" {
		return slot.Name
	}
	return fmt.Sprintf("时段 %d", index+1)
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
