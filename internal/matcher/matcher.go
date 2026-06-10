package matcher

import (
	"strings"

	"github.com/digkill/bitrix-auto-reply/internal/storage"
)

/*
	MatchResult — результат поиска правила.

	Если Found=false — ничего не нашли.
	Если Found=true — нашли правило, которое нужно выполнить.
*/
type MatchResult struct {
	Found bool
	Rule  storage.Rule
}

/*
	Match ищет первое подходящее правило.

	Логика простая:
	- приводим сообщение к нижнему регистру;
	- идём по правилам из БД;
	- если сообщение содержит одно из ключевых слов — правило сработало.

	Пример:
	Сообщение: "А сколько стоит подключение?"
	Ключ: "сколько стоит"
	Правило сработает.
*/
func Match(text string, rules []storage.Rule) MatchResult {
	normalizedText := strings.ToLower(strings.TrimSpace(text))

	if normalizedText == "" {
		return MatchResult{Found: false}
	}

	for _, rule := range rules {
		for _, keyword := range rule.Keywords {
			normalizedKeyword := strings.ToLower(strings.TrimSpace(keyword))

			if normalizedKeyword == "" {
				continue
			}

			if strings.Contains(normalizedText, normalizedKeyword) {
				return MatchResult{
					Found: true,
					Rule:  rule,
				}
			}
		}
	}

	return MatchResult{Found: false}
}