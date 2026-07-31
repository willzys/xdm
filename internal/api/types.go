package api

import "time"

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type Event struct {
	ID             string    `json:"id"`
	EventType      string    `json:"event_type"`
	Text           string    `json:"text"`
	SenderID       string    `json:"sender_id"`
	ConversationID string    `json:"dm_conversation_id"`
	ParticipantIDs []string  `json:"participant_ids"`
	CreatedAt      time.Time `json:"created_at"`
}

type EventPage struct {
	Data     []Event `json:"data"`
	Includes struct {
		Users []User `json:"users"`
	} `json:"includes"`
	Meta struct {
		NextToken   string `json:"next_token"`
		ResultCount int    `json:"result_count"`
	} `json:"meta"`
}

type SendResult struct {
	Data struct {
		ConversationID string `json:"dm_conversation_id"`
		EventID        string `json:"dm_event_id"`
	} `json:"data"`
}
