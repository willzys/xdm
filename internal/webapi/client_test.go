package webapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/willzys/xdm/internal/api"
)

func TestClientImplementsWebDMFlow(t *testing.T) {
	var sentText string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ") {
			t.Error("request did not include the web bearer token")
		}
		if request.Header.Get("X-Twitter-Active-User") != "yes" {
			t.Error("request did not identify an active web user")
		}
		switch request.URL.Path {
		case "/graphql/GetInitialXChatPageQuery":
			if request.Header.Get("X-Client-UUID") == "" {
				t.Error("XChat request did not include a client UUID")
			}
			if request.URL.Query().Get("variables") == "" {
				t.Error("XChat request did not include variables")
			}
			writeJSON(t, response, `{"data":{"get_initial_chat_page":{"items":[{"latest_message_events":["encoded"],"latest_conversation_key_change_events":["key"],"conversation_detail":{"conversation_id":"100:200","participants_results":[{"rest_id":"100","result":{"core":{"name":"Example User","screen_name":"example"}}},{"rest_id":"200","result":{"core":{"name":"Friend","screen_name":"friend"}}}]}}]}}}`)
		case "/graphql/GetPublicKeys":
			if !strings.Contains(request.URL.Query().Get("variables"), `"include_juicebox_tokens":true`) {
				t.Error("public-key request did not request Juicebox tokens")
			}
			writeJSON(t, response, `{"data":{"user_results_by_rest_ids":[{"rest_id":"100","result":{"get_public_keys":{"is_managed_pin_user":true,"public_keys_with_token_map":[{"public_key_with_metadata":{"version":"1","public_key":{"public_key":"identity","signing_public_key":"signing","identity_public_key_signature":"binding"}},"token_map":{"key_store_token_map_json":"{}","token_map":[{"key":"aa","value":{"token":"token-a"}},{"key":"bb","value":{"token":"token-b"}}]}}]}}},{"rest_id":"200","result":{"get_public_keys":{"public_keys_with_token_map":[{"public_key_with_metadata":{"version":"2","public_key":{"public_key":"friend-identity","signing_public_key":"friend-signing","identity_public_key_signature":"friend-binding"}},"token_map":{}}]}}}]}}`)
		case "/1.1/dm/inbox_initial_state.json":
			if request.URL.Query().Get("include_inbox_timelines") != "true" {
				t.Error("inbox request did not include inbox timelines")
			}
			writeJSON(t, response, `{
  "inbox_initial_state": {
    "users": {
      "100": {"id_str":"100","name":"Example User","screen_name":"example"},
      "200": {"id_str":"200","name":"Friend","screen_name":"friend"}
    },
    "conversations": {
      "100-200": {"participants":[{"user_id":"100"},{"user_id":"200"}]}
    },
    "entries": [{
      "message": {
        "id":"900", "time":"1785974400000", "conversation_id":"100-200",
        "message_data":{"id":"900","time":"1785974400000","sender_id":"200","recipient_id":"100","text":"hello"}
      }
    }]
  }
}`)
		case "/1.1/dm/new2.json":
			if request.Method != http.MethodPost {
				t.Errorf("send method = %s, want POST", request.Method)
			}
			if request.Header.Get("Origin") != "https://x.com" {
				t.Error("send request did not include the X origin")
			}
			var payload struct {
				ConversationID string `json:"conversation_id"`
				RequestID      string `json:"request_id"`
				Text           string `json:"text"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.ConversationID != "100-200" {
				t.Errorf("conversation_id = %q", payload.ConversationID)
			}
			if !regexp.MustCompile(`^[0-9A-F]{8}(?:-[0-9A-F]{4}){3}-[0-9A-F]{12}$`).MatchString(payload.RequestID) {
				t.Errorf("request_id = %q, want uppercase UUID", payload.RequestID)
			}
			sentText = payload.Text
			writeJSON(t, response, `{"entries":[{"message":{"id":"901","conversation_id":"100-200","message_data":{"id":"901"}}}]}`)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := newClient(server.URL, server.Client(), api.User{ID: "100", Name: "Example User", Username: "example"})
	user, err := client.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "100" || user.Username != "example" {
		t.Fatalf("Me() = %#v", user)
	}
	page, err := client.Events(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("Events() returned %d events", len(page.Data))
	}
	event := page.Data[0]
	if event.ID != "900" || event.ConversationID != "100-200" || event.SenderID != "200" || event.Text != "hello" {
		t.Fatalf("event = %#v", event)
	}
	wantTime := time.UnixMilli(1785974400000).UTC()
	if !event.CreatedAt.Equal(wantTime) {
		t.Fatalf("event time = %s, want %s", event.CreatedAt, wantTime)
	}
	if strings.Join(event.ParticipantIDs, ",") != "100,200" {
		t.Fatalf("participant IDs = %v", event.ParticipantIDs)
	}
	diagnostics, err := client.DiagnoseInbox(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !diagnostics.HasInitialState || diagnostics.ConversationCount != 1 || diagnostics.EntryCount != 1 || diagnostics.MessageEntryCount != 1 || diagnostics.UserCount != 2 {
		t.Fatalf("DiagnoseInbox() = %#v", diagnostics)
	}
	if diagnostics.EntryKinds["message"] != 1 {
		t.Fatalf("entry kinds = %v", diagnostics.EntryKinds)
	}
	if diagnostics.XChatItemCount != 1 || diagnostics.XChatEventCount != 1 || diagnostics.XChatKeyEventCount != 1 || diagnostics.XChatErrorCount != 0 {
		t.Fatalf("XChat diagnostics = %#v", diagnostics)
	}
	if diagnostics.XChatPublicKeyVersions != 1 || diagnostics.XChatJuiceboxRealms != 2 || !diagnostics.XChatHasJuiceboxConfig || !diagnostics.XChatManagedPIN {
		t.Fatalf("XChat key diagnostics = %#v", diagnostics)
	}
	material, err := client.PrepareXChatUnlock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if material.UserID != "100" || material.KeyVersion != "1" || len(material.Events) != 2 || len(material.SigningKeys) != 2 {
		t.Fatalf("PrepareXChatUnlock() = %#v", material)
	}
	if material.RealmTokens["aa"] != "token-a" || string(material.JuiceboxConfig) == "" {
		t.Fatalf("unlock recovery metadata = %#v", material)
	}
	if len(material.Users) != 2 || strings.Join(material.Participants["100:200"], ",") != "100,200" {
		t.Fatalf("unlock conversation metadata = %#v", material)
	}
	result, err := client.Send(context.Background(), "100-200", "  keep my spacing  ")
	if err != nil {
		t.Fatal(err)
	}
	if result.Data.EventID != "901" || result.Data.ConversationID != "100-200" {
		t.Fatalf("Send() = %#v", result)
	}
	if sentText != "  keep my spacing  " {
		t.Fatalf("sent text = %q", sentText)
	}
}

func TestClientDoesNotExposeUnexpectedErrorBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusForbidden)
		fmt.Fprint(response, "private response content")
	}))
	defer server.Close()

	_, err := newClient(server.URL, server.Client(), api.User{ID: "100", Username: "example"}).Events(context.Background(), "")
	if err == nil {
		t.Fatal("Me() returned no error")
	}
	if strings.Contains(err.Error(), "private response content") {
		t.Fatalf("error exposed response body: %v", err)
	}
}

func TestClientSendsEncryptedXChatMessage(t *testing.T) {
	routedEvent := base64.StdEncoding.EncodeToString(thriftTestStruct(
		thriftTestField(thriftString, 4, thriftTestString("100:200")),
		thriftTestField(thriftString, 5, thriftTestString("conversation-token")),
	))
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/graphql/GetInitialXChatPageQuery":
			writeJSON(t, response, fmt.Sprintf(`{"data":{"get_initial_chat_page":{"items":[{"latest_message_events":[%q],"conversation_detail":{"conversation_id":"100:200","participants_results":[{"rest_id":"100"},{"rest_id":"200"}]}}]}}}`, routedEvent))
		case "/graphql/GetPublicKeys":
			writeJSON(t, response, `{"data":{"user_results_by_rest_ids":[{"rest_id":"100","result":{"get_public_keys":{"public_keys_with_token_map":[{"public_key_with_metadata":{"version":"1","public_key":{"public_key":"identity","signing_public_key":"signing","identity_public_key_signature":"binding"}},"token_map":{"key_store_token_map_json":"{}","token_map":[{"key":"aa","value":{"token":"realm-token"}}]}}]}}}]}}`)
		case "/graphql/SendMessageMutation":
			var payload struct {
				OperationName string `json:"operationName"`
				Variables     struct {
					ConversationID               string `json:"conversation_id"`
					MessageID                    string `json:"message_id"`
					ConversationToken            string `json:"conversation_token"`
					EncodedMessageCreateEvent    string `json:"encoded_message_create_event"`
					EncodedMessageEventSignature string `json:"encoded_message_event_signature"`
				} `json:"variables"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.OperationName != "SendMessageMutation" || payload.Variables.ConversationID != "100:200" || payload.Variables.ConversationToken != "conversation-token" {
				t.Fatalf("send payload = %#v", payload)
			}
			if payload.Variables.MessageID != "message-id" || payload.Variables.EncodedMessageCreateEvent != "encrypted" || payload.Variables.EncodedMessageEventSignature != "signature" {
				t.Fatalf("encrypted send payload = %#v", payload.Variables)
			}
			writeJSON(t, response, `{"data":{"xchat_send_create_message_event":{"encoded_message_event":"confirmed"}}}`)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := newClient(server.URL, server.Client(), api.User{ID: "100", Username: "example"})
	client.SetXChatDecryptor(fakeXChatCrypto{})
	result, err := client.Send(context.Background(), "100:200", "hello encrypted")
	if err != nil {
		t.Fatal(err)
	}
	if result.Data.EventID != "message-id" || result.Data.ConversationID != "100:200" {
		t.Fatalf("Send() = %#v", result)
	}
}

type fakeXChatCrypto struct{}

func (fakeXChatCrypto) Decrypt(context.Context, XChatUnlockMaterial) (api.EventPage, error) {
	return api.EventPage{}, nil
}

func (fakeXChatCrypto) EncryptMessage(context.Context, string, string) (XChatEncryptedMessage, error) {
	return XChatEncryptedMessage{
		MessageID: "message-id", EncodedMessageCreateEvent: "encrypted", EncodedMessageEventSignature: "signature",
	}, nil
}

func writeJSON(t *testing.T, response http.ResponseWriter, value string) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	if _, err := response.Write([]byte(value)); err != nil {
		t.Fatal(err)
	}
}
