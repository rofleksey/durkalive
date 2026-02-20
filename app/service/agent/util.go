package agent

import "time"

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "никогда"
	}

	return t.Format("15:04:05")
}
