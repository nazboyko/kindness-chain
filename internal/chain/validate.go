package chain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits on what a link can hold. They keep every memo well inside one
// transaction and keep the feed readable.
const (
	minActLength = 10
	maxActLength = 200
	maxByLength  = 40
)

// The refusals, worded for the visitor. The frontend repeats the length
// ones so it can answer before a request is made.
const (
	msgTooShort      = "Write at least 10 characters. One honest sentence is enough."
	msgTooLong       = "Keep it to 200 characters. One sentence is enough."
	msgNameTooLong   = "Keep your name to 40 characters."
	msgNoLinks       = "Links are not allowed here, just the sentence."
	msgBlockedWord   = "That word is not welcome here. Try another sentence."
	msgTooBigOnChain = "That sentence is too long to fit on-chain in one memo. Try fewer characters."
	msgDuplicate     = "This sentence is already in the chain."
)

// ValidationError explains, in the visitor's words, why a submission
// was refused.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func refuse(message string) error {
	return &ValidationError{Message: message}
}

// ErrDuplicate is the refusal for a sentence the chain already has. The
// API checks for duplicates itself, before any limit is charged.
var ErrDuplicate = &ValidationError{Message: msgDuplicate}

// Validate cleans a submission and refuses what the chain should not
// carry. It returns the cleaned sentence and name.
func Validate(act, by string) (string, string, error) {
	act = normalize(act)
	by = normalize(by)
	switch {
	case utf8.RuneCountInString(act) < minActLength:
		return "", "", refuse(msgTooShort)
	case utf8.RuneCountInString(act) > maxActLength:
		return "", "", refuse(msgTooLong)
	case utf8.RuneCountInString(by) > maxByLength:
		return "", "", refuse(msgNameTooLong)
	case hasLink(act) || hasLink(by):
		return "", "", refuse(msgNoLinks)
	case hasBlockedWord(act) || hasBlockedWord(by):
		return "", "", refuse(msgBlockedWord)
	}
	return act, by, nil
}

// normalize trims, collapses runs of whitespace into one space, and
// drops control and invisible characters.
func normalize(s string) string {
	var b strings.Builder
	pendingSpace := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			pendingSpace = true
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), r == utf8.RuneError:
			// nothing a person meant to write
		default:
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fingerprint reduces a sentence to what a duplicate check compares:
// lowercase, no punctuation or symbols, single spaces. Two sentences
// that differ only in those ways are the same sentence.
func fingerprint(act string) string {
	var b strings.Builder
	pendingSpace := false
	for _, r := range strings.ToLower(normalize(act)) {
		switch {
		case unicode.IsSpace(r):
			pendingSpace = true
		case unicode.IsPunct(r), unicode.IsSymbol(r):
			// punctuation does not make a different sentence
		default:
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			pendingSpace = false
			b.WriteRune(r)
		}
	}
	return b.String()
}

var linkMarkers = []string{"http://", "https://", "www."}

func hasLink(s string) bool {
	lower := strings.ToLower(s)
	for _, marker := range linkMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// blocked is short and dumb on purpose. It matches whole words only, so
// ordinary words that happen to contain these letters pass.
var blocked = map[string]bool{
	"fuck": true, "fucking": true, "fucked": true, "fucker": true, "fuckers": true,
	"shit": true, "shitty": true, "bullshit": true,
	"bitch": true, "bitches": true,
	"asshole": true, "assholes": true,
	"cunt": true, "cunts": true,
	"nigger": true, "niggers": true, "nigga": true, "niggas": true,
	"faggot": true, "faggots": true, "fag": true, "fags": true,
	"retard": true, "retarded": true, "retards": true,
	"whore": true, "whores": true, "slut": true, "sluts": true,
}

func hasBlockedWord(s string) bool {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	for _, word := range words {
		if blocked[word] {
			return true
		}
	}
	return false
}
