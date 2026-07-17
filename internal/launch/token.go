package launch

import (
	"fmt"
	"log/slog"
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
// Не включает хвост токена: только факт наличия и длина (Phase 4 hardening).
func TokenHint(token string) string {
	n := len(token)
	if n == 0 {
		return "empty"
	}
	return "set(len=" + strconv.Itoa(n) + ")"
}

// IsEnvTokenRef — token_ref ссылается на ENV (env:NAME / $NAME), а не plaintext.
func IsEnvTokenRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	return strings.HasPrefix(ref, "env:") || strings.HasPrefix(ref, "$")
}

// MaskTokenRef — значение для API/ctl list: env-ссылки оставляем как есть,
// plaintext маскируем (не светим полный секрет в выводе).
func MaskTokenRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if IsEnvTokenRef(ref) {
		return ref
	}
	n := len(ref)
	if n <= 4 {
		return "***"
	}
	// Префикс 2 + маска + хвост 2 — отличить записи, не раскрывая середину.
	return ref[:2] + strings.Repeat("*", min(8, n-4)) + ref[n-2:]
}

// WarnIfPlaintextTokenRef пишет slog.Warn, если ref выглядит как сырой токен
// (рекомендуется env:NAME). Полное значение в лог не попадает.
func WarnIfPlaintextTokenRef(log *slog.Logger, botID, ref string) {
	if IsEnvTokenRef(ref) || strings.TrimSpace(ref) == "" {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	log.Warn("token_ref выглядит как plaintext; предпочтительно env:NAME или $NAME",
		"bot_id", botID,
		"token_hint", TokenHint(ref),
	)
}
