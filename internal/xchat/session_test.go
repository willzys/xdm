package xchat

import (
	"testing"

	"github.com/willzys/xdm/internal/webapi"
)

func TestResponseEventPageKeepsValidMessagesWhenOneEventFails(t *testing.T) {
	material := webapi.XChatUnlockMaterial{
		Events:       []string{"key", "message", "unsupported"},
		Participants: map[string][]string{"100:200": {"100", "200"}},
	}
	response := sessionResponse{Errors: 1, MessageEvents: 1}
	response.Events = append(response.Events, struct {
		ID             string `json:"id"`
		SenderID       string `json:"senderId"`
		ConversationID string `json:"conversationId"`
		CreatedAtMsec  int64  `json:"createdAtMsec"`
		Text           string `json:"text"`
		Verified       bool   `json:"verified"`
	}{ID: "message", SenderID: "200", ConversationID: "100:200", CreatedAtMsec: 1, Text: "hello", Verified: true})

	page, err := responseEventPage(material, response)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0].Text != "hello" {
		t.Fatalf("responseEventPage() = %#v", page)
	}
}

func TestResponseEventPageFailsWhenNoMessagesDecrypt(t *testing.T) {
	_, err := responseEventPage(webapi.XChatUnlockMaterial{Events: []string{"message"}}, sessionResponse{Errors: 1})
	if err == nil {
		t.Fatal("responseEventPage() returned no error")
	}
}
