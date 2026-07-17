package launch

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"mvp-manager/internal/store"
)

// ResolveTokenRef превращает bots.token_ref в реальный токен мессенджера.
//
// MVP-правила (без Vault):
//   - "env:NAME" или "$NAME" — значение из переменной окружения NAME;
//   - иначе token_ref используется как сам токен (как в Phase 1 ctl --token).
//
// Пустой ref / пустой ENV → ErrInvalidArgument. Полный токен в логи не пишем
// (см. TokenHint).
func ResolveTokenRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty token_ref: %w", store.ErrInvalidArgument)
	}

	var envName string
	switch {
	case strings.HasPrefix(ref, "env:"):
		envName = strings.TrimSpace(strings.TrimPrefix(ref, "env:"))
	case strings.HasPrefix(ref, "$"):
		envName = strings.TrimSpace(strings.TrimPrefix(ref, "$"))
	default:
		// Прямое значение токена (MVP).
		return ref, nil
	}

	if envName == "" {
		return "", fmt.Errorf("empty env name in token_ref %q: %w", ref, store.ErrInvalidArgument)
	}
	val, ok := os.LookupEnv(envName)
	if !ok || strings.TrimSpace(val) == "" {
		return "", fmt.Errorf("env %q for token_ref is empty or unset: %w", envName, store.ErrInvalidArgument)
	}
	return val, nil
}

// TokenHint — безопасная строка для slog (без полного секрета).
func TokenHint(token string) string {
	n := len(token)
	if n == 0 {
		return "empty"
	}
	if n <= 4 {
		return "set(len=" + strconv.Itoa(n) + ")"
	}
	// Только длина + 2 последних символа — достаточно отличить ботов в логах.
	return "set(len=" + strconv.Itoa(n) + ",…)" + token[n-2:]
}
