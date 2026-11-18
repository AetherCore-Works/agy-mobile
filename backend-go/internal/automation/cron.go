package automation

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronSchedule holds parsed 5-field Cron matching masks (minute, hour, dom, month, dow).
type CronSchedule struct {
	Expr    string
	minutes [60]bool
	hours   [24]bool
	doms    [32]bool // 1..31
	months  [13]bool // 1..12
	dows    [7]bool  // 0..6 (0=Sunday)
	domWild bool
	dowWild bool
}

var cronAliases = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

func parseField(expr string, minVal, maxVal int, wrapSeven bool, dest []bool) (bool, error) {
	raw := strings.TrimSpace(expr)
	if raw == "" {
		return false, fmt.Errorf("campo cron vazio")
	}
	wildcard := raw == "*"
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return false, fmt.Errorf("item vazio em %q", expr)
		}
		rangeStr, stepStr, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(strings.TrimSpace(stepStr))
			if err != nil || n <= 0 {
				return false, fmt.Errorf("step inválido em %q", part)
			}
			step = n
		}

		var start, end int
		if rangeStr == "*" {
			start = minVal
			end = maxVal
		} else if loStr, hiStr, hasDash := strings.Cut(rangeStr, "-"); hasDash {
			lo, err1 := strconv.Atoi(strings.TrimSpace(loStr))
			hi, err2 := strconv.Atoi(strings.TrimSpace(hiStr))
			if err1 != nil || err2 != nil || lo > hi {
				return false, fmt.Errorf("intervalo inválido em %q", part)
			}
			start, end = lo, hi
		} else {
			v, err := strconv.Atoi(strings.TrimSpace(rangeStr))
			if err != nil {
				return false, fmt.Errorf("valor inválido em %q", part)
			}
			start = v
			if hasStep {
				end = maxVal
			} else {
				end = v
			}
		}

		for v := start; v <= end; v += step {
			actual := v
			if wrapSeven && actual == 7 {
				actual = 0
			}
			if actual < minVal || actual > maxVal {
				return false, fmt.Errorf("valor %d fora dos limites [%d..%d]", v, minVal, maxVal)
			}
			dest[actual] = true
		}
	}
	return wildcard, nil
}

// ParseCron validates and compiles a standard 5-field Cron expression or @alias.
func ParseCron(expr string) (*CronSchedule, error) {
	norm := strings.TrimSpace(expr)
	if norm == "" {
		return nil, fmt.Errorf("expressão cron vazia")
	}
	if alias, ok := cronAliases[strings.ToLower(norm)]; ok {
		norm = alias
	}
	fields := strings.Fields(norm)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron deve ter 5 campos (min hora dia mês semana), ex: \"0 8 * * *\"")
	}

	s := &CronSchedule{Expr: strings.TrimSpace(expr)}
	if _, err := parseField(fields[0], 0, 59, false, s.minutes[:]); err != nil {
		return nil, fmt.Errorf("minuto: %w", err)
	}
	if _, err := parseField(fields[1], 0, 23, false, s.hours[:]); err != nil {
		return nil, fmt.Errorf("hora: %w", err)
	}
	domW, err := parseField(fields[2], 1, 31, false, s.doms[:])
	if err != nil {
		return nil, fmt.Errorf("dia do mês: %w", err)
	}
	s.domWild = domW
	if _, err := parseField(fields[3], 1, 12, false, s.months[:]); err != nil {
		return nil, fmt.Errorf("mês: %w", err)
	}
	dowW, err := parseField(fields[4], 0, 6, true, s.dows[:])
	if err != nil {
		return nil, fmt.Errorf("dia da semana: %w", err)
	}
	s.dowWild = dowW
	return s, nil
}

// Matches reports whether t (truncated to minute) satisfies the Cron schedule.
func (s *CronSchedule) Matches(t time.Time) bool {
	if !s.minutes[t.Minute()] || !s.hours[t.Hour()] || !s.months[int(t.Month())] {
		return false
	}
	domMatch := s.doms[t.Day()]
	dowMatch := s.dows[int(t.Weekday())]
	// Standard Vixie cron: if both DOM and DOW are restricted, either matching triggers.
	if !s.domWild && !s.dowWild {
		return domMatch || dowMatch
	}
	return domMatch && dowMatch
}

// NextAfter computes the next matching minute strictly after from, scanning up to 366 days ahead.
func (s *CronSchedule) NextAfter(from time.Time) (time.Time, bool) {
	cand := from.Truncate(time.Minute).Add(time.Minute)
	const maxMinutes = 366 * 24 * 60
	for i := 0; i < maxMinutes; i++ {
		if s.Matches(cand) {
			return cand, true
		}
		cand = cand.Add(time.Minute)
	}
	return time.Time{}, false
}
