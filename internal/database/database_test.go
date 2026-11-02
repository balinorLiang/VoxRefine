package database

import (
	"context"
	"testing"
)

func TestOpenRejectsInvalidDatabaseURL(t *testing.T) {
	pool, err := Open(context.Background(), "://not-a-url", 2, 0)
	if err == nil {
		pool.Close()
		t.Fatal("expected invalid URL error")
	}
}
