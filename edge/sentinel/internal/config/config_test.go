package config

import (
	"strings"
	"testing"
)

const (
	tomToken   = "0123456789abcdef"
	mariaToken = "fedcba9876543210"
)

func TestParsePeople(t *testing.T) {
	// The server as it has always been configured: one token, no names.
	people, err := parsePeople("owner", tomToken, "")
	if err != nil || len(people) != 1 || people[0].ID != "owner" || people[0].Token != tomToken {
		t.Fatalf("single person: %+v err=%v", people, err)
	}
	people, err = parsePeople("tom", tomToken, " maria:"+mariaToken+" , guest:aaaaaaaaaaaaaaaaaaaa,")
	if err != nil || len(people) != 3 {
		t.Fatalf("three people: %+v err=%v", people, err)
	}
	if people[0].ID != "tom" || people[1].ID != "maria" || people[1].Token != mariaToken || people[2].ID != "guest" {
		t.Fatalf("order and values: %+v", people)
	}
	// Only the first colon separates: a token may contain one.
	if p, err := parsePeople("tom", tomToken, "maria:abc:"+mariaToken); err != nil || p[1].Token != "abc:"+mariaToken {
		t.Fatalf("token with a colon: %+v err=%v", p, err)
	}
	if people[1].Name() != "Maria" {
		t.Fatalf("name %q", people[1].Name())
	}

	bad := map[string][3]string{
		"same token as the owner": {"tom", tomToken, "maria:" + tomToken},
		"same id twice":           {"tom", tomToken, "tom:" + mariaToken},
		"short token":             {"tom", tomToken, "maria:short"},
		"no token":                {"tom", tomToken, "maria"},
		"uppercase id":            {"Tom", tomToken, ""},
		"id with a space in it":   {"tom", tomToken, "the wife:" + mariaToken},
		"empty owner":             {"", tomToken, ""},
	}
	for name, c := range bad {
		if _, err := parsePeople(c[0], c[1], c[2]); err == nil {
			t.Errorf("%s: accepted", name)
		} else if strings.Contains(err.Error(), tomToken) || strings.Contains(err.Error(), mariaToken) {
			t.Errorf("%s: error message contains a token", name)
		}
	}
}
