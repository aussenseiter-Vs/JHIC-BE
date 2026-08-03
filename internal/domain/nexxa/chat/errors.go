package chat

import "errors"

var (
	ErrChatMessageRequired = errors.New("chat message is required")
	ErrChatMessageTooLong  = errors.New("chat message exceeds 300 characters")
)