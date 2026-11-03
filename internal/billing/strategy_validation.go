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

// slotSegment is one continuous minute range of a slot, attributed to the day it
// actually falls on:
//
//	dayOffset 0 - the configured start day (evening half of a cross-midnight slot)
//	dayOffset 1 - the following day (after-midnight half of a cross-midnight slot)
//
// Overlap detection compares segments on their *effective* weekday, so a
// cross-midnight slot's tail can be checked against a slot configured for the next
// day. A shared same-day model (the previous behaviour) both missed real overlaps
// across the midnight boundary and reported overlaps that can never happen.
type slotSegment struct {
	dayOffset int
	start     int
	end       int
}

// slotSegments expands a validated slot into the minute ranges it covers.
func slotSegments(slot OffPeakSlot) ([]slotSegment, error) {
	startM, err := parseClockMinutes(slot.Start)
	if err != nil {
		return nil, err
	}
	endM, err := parseClockMinutes(slot.End)
	if err != nil {
		return nil, err
	}
	if startM < endM {
		return []slotSegment{{dayOffset: 0, start: startM, end: endM}}, nil
	}
	// Cross midnight: [start, 24:00) on the start day and [00:00, end) the next day.
	return []slotSegment{
		{dayOffset: 0, start: startM, end: 1440},
		{dayOffset: 1, start: 0, end: endM},
	}, nil
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

	dayNames := map[int]string{1: "周一", 2: "周二", 3: "周三", 4: "周四", 5: "周五", 6: "周六", 7: "周日"}

	for i := 0; i < len(slots); i++ {
		s1 := slots[i]
		segments1, err := slotSegments(s1)
		if err != nil {
			return fmt.Errorf("[%s] %w", offPeakSlotName(s1, i), err)
		}
		days1 := s1.Days
		if len(days1) == 0 {
			days1 = []int{1, 2, 3, 4, 5, 6, 7}
		}

		for j := i + 1; j < len(slots); j++ {
			s2 := slots[j]
			segments2, err := slotSegments(s2)
			if err != nil {
				return fmt.Errorf("[%s] %w", offPeakSlotName(s2, j), err)
			}
			days2 := s2.Days
			if len(days2) == 0 {
				days2 = []int{1, 2, 3, 4, 5, 6, 7}
			}

			// Compare every (start day, segment) combination on its effective
			// weekday. A segment that runs past midnight is evaluated on the next
			// day, exactly like the pricing path does (slotCoversInstant).
			var overlapDays []int
			for _, d1 := range days1 {
				for _, seg1 := range segments1 {
					effective1 := shiftISODay(d1, seg1.dayOffset)
					for _, d2 := range days2 {
						for _, seg2 := range segments2 {
							if effective1 != shiftISODay(d2, seg2.dayOffset) {
								continue
							}
							maxS := seg1.start
							if seg2.start > maxS {
								maxS = seg2.start
							}
							minE := seg1.end
							if seg2.end < minE {
								minE = seg2.end
							}
							if maxS < minE && !containsDay(overlapDays, effective1) {
								overlapDays = append(overlapDays, effective1)
							}
						}
					}
				}
			}

			if len(overlapDays) > 0 {
				sortDays(overlapDays)
				var dayStrs []string
				for _, d := range overlapDays {
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

// shiftISODay moves an ISO weekday (1=Mon … 7=Sun) forward by offset days.
func shiftISODay(isoDay, offset int) int {
	shifted := isoDay + offset
	for shifted > 7 {
		shifted -= 7
	}
	return shifted
}

// containsDay reports whether days already holds isoDay.
func containsDay(days []int, isoDay int) bool {
	for _, d := range days {
		if d == isoDay {
			return true
		}
	}
	return false
}

// sortDays sorts ISO weekdays ascending (insertion sort: the list has at most 7).
func sortDays(days []int) {
	for i := 1; i < len(days); i++ {
		for j := i; j > 0 && days[j] < days[j-1]; j-- {
			days[j], days[j-1] = days[j-1], days[j]
		}
	}
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
	startM, err := parseClockMinutes(slot.Start)
	if err != nil {
		return fmt.Errorf("[%s] 开始时间%w", name, err)
	}
	endM, err := parseClockMinutes(slot.End)
	if err != nil {
		return fmt.Errorf("[%s] 结束时间%w", name, err)
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

// parseClockMinutes parses a strict 24-hour clock value: one or two digits for the
// hour, exactly two for the minute, 00:00-23:59. Anything else - an AM/PM suffix,
// a non-numeric part, a missing field, an out-of-range value - is an error.
//
// The lenient parser this replaces returned 0 for every parse failure, so "8:30pm"
// became 08:00 and "abc" became midnight, and validateOffPeakSlot's range check
// accepted both: a typo silently moved a discount window instead of failing the
// save.
func parseClockMinutes(value string) (int, error) {
	raw := strings.TrimSpace(value)
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("格式不正确，需为 24 小时制 HH:MM: %q", value)
	}
	hourPart, minutePart := parts[0], parts[1]
	if !isASCIIDigits(hourPart) || len(hourPart) > 2 || len(minutePart) != 2 || !isASCIIDigits(minutePart) {
		return 0, fmt.Errorf("格式不正确，需为 24 小时制 HH:MM: %q", value)
	}
	hour, _ := strconv.Atoi(hourPart)
	minute, _ := strconv.Atoi(minutePart)
	if hour > 23 || minute > 59 {
		return 0, fmt.Errorf("超出范围，需在 00:00-23:59 之间: %q", value)
	}
	return hour*60 + minute, nil
}

// isASCIIDigits reports whether s is a non-empty run of ASCII digits.
func isASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// parseRowWindow resolves the row-level off-peak start/end fields.
//
// Empty fields mean "no configured window" and resolve to 0/0 with ok=true: the
// night strategy then substitutes its historical 00:00-08:30 default and the
// window strategy simply never matches. A malformed value is never coerced -
// ok=false lets the caller charge the undiscounted price instead of inventing a
// window out of a typo.
func parseRowWindow(start, end string) (startM, endM int, ok bool) {
	rawStart, rawEnd := strings.TrimSpace(start), strings.TrimSpace(end)
	if rawStart == "" || rawEnd == "" {
		return 0, 0, rawStart == "" && rawEnd == ""
	}
	startM, err := parseClockMinutes(rawStart)
	if err != nil {
		return 0, 0, false
	}
	endM, err = parseClockMinutes(rawEnd)
	if err != nil {
		return 0, 0, false
	}
	return startM, endM, true
}
