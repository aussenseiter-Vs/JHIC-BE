package chat_test

import (
	"testing"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/chat"
	"github.com/stretchr/testify/require"
)

func TestMemory_AppendAndMessages(t *testing.T) {
	mem := chat.NewMemory(4, time.Hour)

	for i := 1; i <= 5; i++ {
		mem.Append("s1", "q", "a")
	}
	msgs := mem.Messages("s1")
	require.Len(t, msgs, 4)
	require.Equal(t, "user", msgs[0].Role)
	require.Equal(t, "assistant", msgs[3].Role)
}

func TestMemory_SessionsIndependent(t *testing.T) {
	mem := chat.NewMemory(20, time.Hour)
	mem.Append("s1", "halo", "hai")
	mem.Append("s2", "selamat pagi", "pagi")
	require.Len(t, mem.Messages("s1"), 2)
	require.Len(t, mem.Messages("s2"), 2)
	require.Equal(t, "halo", mem.Messages("s1")[0].Content)
}

func TestMemory_ExpiresAfterTTL(t *testing.T) {
	mem := chat.NewMemory(20, 50*time.Millisecond)
	mem.Append("s1", "halo", "hai")
	time.Sleep(60 * time.Millisecond)
	require.Empty(t, mem.Messages("s1"))

	mem.Append("s1", "lagi", "ok")
	require.Len(t, mem.Messages("s1"), 2)
}

func TestMemory_ReturnsCopy(t *testing.T) {
	mem := chat.NewMemory(20, time.Hour)
	mem.Append("s1", "halo", "hai")

	got := mem.Messages("s1")
	got[0].Content = "mutated"
	require.Equal(t, "halo", mem.Messages("s1")[0].Content)
}