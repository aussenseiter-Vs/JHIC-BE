package match

import "errors"

var (
	ErrAnswersRequired    = errors.New("all 8 answers are required")
	ErrAnswerTooLong      = errors.New("each answer must be at most 500 characters")
	ErrNexxaOutputInvalid = errors.New("AI output could not be interpreted")
)