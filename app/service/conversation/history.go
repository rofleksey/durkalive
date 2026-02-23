package conversation

import (
	"fmt"
	"strings"
	"time"
)

const messageHistorySize = 20

type chatMessage struct {
	Username  string
	Text      string
	Timestamp time.Time
}

type ChatHistory struct {
	messages []chatMessage
}

func (h *ChatHistory) add(username, text string) {
	msg := chatMessage{
		Username:  username,
		Text:      text,
		Timestamp: time.Now(),
	}

	if len(h.messages) >= messageHistorySize {
		h.messages = append(h.messages[1:], msg)
	} else {
		h.messages = append(h.messages, msg)
	}
}

func (h *ChatHistory) format(limit ...int) string {
	if len(h.messages) == 0 {
		return "No recent messages"
	}

	messagesToShow := len(h.messages)
	if len(limit) > 0 && limit[0] > 0 && limit[0] < messagesToShow {
		messagesToShow = limit[0]
	}

	var builder strings.Builder

	startIdx := len(h.messages) - messagesToShow
	if startIdx < 0 {
		startIdx = 0
	}

	for i := startIdx; i < len(h.messages); i++ {
		msg := h.messages[i]
		builder.WriteString(fmt.Sprintf("%s - %s: %s\n", formatTime(msg.Timestamp), msg.Username, msg.Text))
	}

	return builder.String()
}
