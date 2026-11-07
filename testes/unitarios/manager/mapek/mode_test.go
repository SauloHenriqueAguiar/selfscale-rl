package mapek_test

import (
	"testing"

	"github.com/shna/selfscale-rl/manager/mapek"
)

func TestParseMode(t *testing.T) {
	cases := []struct {
		name       string
		learn, act bool
		wantErr    bool
	}{
		{"observe", false, false, false},
		{"eval", false, true, false},
		{"finetune", true, true, false},
		{"online", true, true, false},
		{"turbo", false, false, true},
	}
	for _, c := range cases {
		m, err := mapek.ParseMode(c.name)
		if (err != nil) != c.wantErr || m.Learn != c.learn || m.Act != c.act {
			t.Errorf("ParseMode(%q) = %+v, %v", c.name, m, err)
		}
	}
}
