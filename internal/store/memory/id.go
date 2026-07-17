package memory

import (
	"crypto/rand"
	"fmt"
)

// newID генерирует UUID v4 в каноническом текстовом виде RFC 4122
// без внешних зависимостей (только crypto/rand из stdlib).
//
// Используется при Create runtime/bot, если вызывающий передал пустой ID.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	// Версия 4 и variant RFC 4122.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
