package webapi

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/willzys/xdm/internal/api"
)

type inboxResponse struct {
	State struct {
		Conversations map[string]struct {
			Participants []struct {
				UserID string `json:"user_id"`
			} `json:"participants"`
		} `json:"conversations"`
		Entries []struct {
			Message *webMessage `json:"message"`
		} `json:"entries"`
		Users map[string]webUser `json:"users"`
	} `json:"inbox_initial_state"`
}

type xchatInboxResponse struct {
	Data struct {
		Page struct {
			Items  []xchatInboxItem  `json:"items"`
			Errors []json.RawMessage `json:"errors"`
		} `json:"get_initial_chat_page"`
	} `json:"data"`
	Errors []json.RawMessage `json:"errors"`
}

type xchatInboxItem struct {
	LatestMessageEvents               []string `json:"latest_message_events"`
	EncodedMessageEvents              []string `json:"encoded_message_events"`
	LatestConversationKeyChangeEvents []string `json:"latest_conversation_key_change_events"`
	ConversationDetail                struct {
		ConversationID string            `json:"conversation_id"`
		Participants   []xchatUserResult `json:"participants_results"`
		GroupAdmins    []xchatUserResult `json:"group_admins_results"`
		GroupMembers   []xchatUserResult `json:"group_members_results"`
		RemovedUsers   []xchatUserResult `json:"group_removed_users"`
	} `json:"conversation_detail"`
}

type xchatUserResult struct {
	RestID string `json:"rest_id"`
	Result *struct {
		Core *struct {
			Name       string `json:"name"`
			ScreenName string `json:"screen_name"`
		} `json:"core"`
	} `json:"result"`
}

type xchatPublicKeysResponse struct {
	Data struct {
		Users []xchatPublicKeysUser `json:"user_results_by_rest_ids"`
	} `json:"data"`
	Errors []json.RawMessage `json:"errors"`
}

type xchatPublicKeysUser struct {
	RestID string `json:"rest_id"`
	Result struct {
		PublicKeys struct {
			Items      []xchatPublicKeyItem `json:"public_keys_with_token_map"`
			ManagedPIN bool                 `json:"is_managed_pin_user"`
		} `json:"get_public_keys"`
	} `json:"result"`
}

type xchatPublicKeyItem struct {
	Metadata struct {
		Key struct {
			IdentityPublicKeySignature string `json:"identity_public_key_signature"`
			PublicKey                  string `json:"public_key"`
			SigningPublicKey           string `json:"signing_public_key"`
		} `json:"public_key"`
		Version string `json:"version"`
	} `json:"public_key_with_metadata"`
	TokenMap xchatTokenMap `json:"token_map"`
}

type xchatTokenMap struct {
	ConfigJSON string `json:"key_store_token_map_json"`
	MaxGuesses int    `json:"max_guess_count"`
	Tokens     []struct {
		Key   string `json:"key"`
		Value struct {
			Token string `json:"token"`
		} `json:"value"`
	} `json:"token_map"`
}

type webMessage struct {
	ID             string `json:"id"`
	Time           string `json:"time"`
	ConversationID string `json:"conversation_id"`
	Data           struct {
		ID        string `json:"id"`
		Time      string `json:"time"`
		SenderID  string `json:"sender_id"`
		Recipient string `json:"recipient_id"`
		Text      string `json:"text"`
	} `json:"message_data"`
}

type webUser struct {
	ID       string `json:"id_str"`
	Name     string `json:"name"`
	Username string `json:"screen_name"`
}

func (r inboxResponse) eventPage() (api.EventPage, error) {
	var page api.EventPage
	page.Includes.Users = make([]api.User, 0, len(r.State.Users))
	for key, user := range r.State.Users {
		if user.ID == "" {
			user.ID = key
		}
		if user.ID == "" {
			continue
		}
		page.Includes.Users = append(page.Includes.Users, api.User{ID: user.ID, Name: user.Name, Username: user.Username})
	}
	for _, entry := range r.State.Entries {
		if entry.Message == nil {
			continue
		}
		message := entry.Message
		conversationID := strings.TrimSpace(message.ConversationID)
		if conversationID == "" {
			continue
		}
		id := message.Data.ID
		if id == "" {
			id = message.ID
		}
		created := message.Data.Time
		if created == "" {
			created = message.Time
		}
		participants := r.State.Conversations[conversationID].Participants
		participantIDs := make([]string, 0, len(participants))
		for _, participant := range participants {
			if participant.UserID != "" {
				participantIDs = append(participantIDs, participant.UserID)
			}
		}
		page.Data = append(page.Data, api.Event{
			ID: id, EventType: "MessageCreate", Text: message.Data.Text,
			SenderID: message.Data.SenderID, ConversationID: conversationID,
			ParticipantIDs: participantIDs, CreatedAt: parseWebTime(created),
		})
	}
	page.Meta.ResultCount = len(page.Data)
	if len(r.State.Conversations) > 0 && len(page.Data) == 0 {
		return api.EventPage{}, errors.New("X web inbox contained conversations but no supported message entries")
	}
	return page, nil
}

type sendResponse struct {
	Entries []struct {
		Message *webMessage `json:"message"`
	} `json:"entries"`
	Event struct {
		ID string `json:"id"`
	} `json:"event"`
}

func (r sendResponse) result(conversationID string) (api.SendResult, error) {
	var result api.SendResult
	result.Data.ConversationID = conversationID
	for _, entry := range r.Entries {
		if entry.Message == nil {
			continue
		}
		if entry.Message.ConversationID != "" {
			result.Data.ConversationID = entry.Message.ConversationID
		}
		result.Data.EventID = entry.Message.Data.ID
		if result.Data.EventID == "" {
			result.Data.EventID = entry.Message.ID
		}
		if result.Data.EventID != "" {
			return result, nil
		}
	}
	result.Data.EventID = r.Event.ID
	if result.Data.EventID == "" {
		return api.SendResult{}, errors.New("X web send response did not include a DM event ID")
	}
	return result, nil
}
