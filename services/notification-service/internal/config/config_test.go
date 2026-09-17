package config

import "testing"

func TestOptions(t *testing.T) {
	c, e := LoadOptions()
	if e != nil || c.PageSize != 30 {
		t.Fatal(c, e)
	}
	t.Setenv("WS_PING_INTERVAL", "60s")
	if _, e := LoadOptions(); e == nil {
		t.Fatal("invalid ping accepted")
	}
	t.Setenv("WS_PING_INTERVAL", "25s")
	t.Setenv("NOTIFICATION_RETRY_IDLE", "1s")
	if _, e := LoadOptions(); e == nil {
		t.Fatal("unsafe retry lease")
	}
}
