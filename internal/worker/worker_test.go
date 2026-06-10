package worker

import "testing"

func TestRequiresMention(t *testing.T) {
	if requiresMention("user") {
		t.Fatal("user dialog must not require mention")
	}

	if !requiresMention("chat") {
		t.Fatal("chat dialog must require mention")
	}
}

func TestMentionsUser(t *testing.T) {
	if !mentionsUser("[USER=160591]BOT ERP[/USER] привет", 160591) {
		t.Fatal("expected Bitrix user mention to match")
	}

	if mentionsUser("[USER=160591]BOT ERP[/USER] привет", 16059) {
		t.Fatal("expected partial id not to match")
	}
}
