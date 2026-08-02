package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/database"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/id"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

type seedUser struct {
	email    string
	password string
	name     string
	role     string
	class    string
	jurusan  string
	position string
}

type seedBerita struct {
	authorIdx     int
	title         string
	content       string
	isAchievement bool
}

func main() {
	exe, _ := os.Executable()
	_ = godotenv.Load(filepath.Join(filepath.Dir(exe), ".env"))
	_ = godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	users := []seedUser{
		{email: "admin@jhic.com", password: "admin123", name: "Admin User", role: "admin"},
		{email: "jurnal@jhic.com", password: "jurnal123", name: "Jurnalis User", role: "jurnal"},
		{email: "user@jhic.com", password: "user123", name: "Regular User", role: "user"},
		{email: "siswa-pplg1@jhic.com", password: "siswa123", name: "Siswa PPLG 1", role: "user", class: "PPLG 1", jurusan: "PPLG"},
	}

	for _, jurusan := range []string{"PPLG", "AK", "HTL"} {
		for i := 1; i <= jurusanClasses(jurusan); i++ {
			class := fmt.Sprintf("%s %d", jurusan, i)
			users = append(users, seedUser{
				email:    slugify(class) + "@jhic.com",
				password: "guru123",
				name:     "Wali Kelas " + class,
				role:     "guru",
				class:    class,
				position: "wali_kelas",
			})
		}
		users = append(users, seedUser{
			email:    "kaprog-" + jurusan + "@jhic.com",
			password: "guru123",
			name:     "Kaprog " + jurusan,
			role:     "guru",
			jurusan:  jurusan,
			position: "kaprog",
		})
	}
	users = append(users,
		seedUser{email: "bk@jhic.com", password: "guru123", name: "Guru BK", role: "guru", position: "bk"},
		seedUser{email: "kesiswaan@jhic.com", password: "guru123", name: "Guru Kesiswaan", role: "guru", position: "kesiswaan"},
	)

	var userIDs []id.ID
	for _, u := range users {
		uid := id.New()
		hash, err := bcrypt.GenerateFromPassword([]byte(u.password), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("hash password for %s: %v", u.email, err)
		}
		now := time.Now().UTC()
		_, err = pool.Exec(ctx,
			`INSERT INTO users (id, email, password_hash, name, role, class, jurusan, position, avatar_url, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '', $9, $10)
			 ON CONFLICT (email) DO UPDATE SET name = $4, role = $5, class = $6, jurusan = $7, position = $8, password_hash = $3, updated_at = $10`,
			uid, u.email, string(hash), u.name, u.role, u.class, u.jurusan, u.position, now, now,
		)
		if err != nil {
			log.Fatalf("insert user %s: %v", u.email, err)
		}
		userIDs = append(userIDs, uid)
		fmt.Printf("user: %s / %s (%s) [%s %s %s]\n", u.email, u.password, uid, u.role, u.class, u.position)
	}

	for _, uid := range userIDs {
		tb := make([]byte, 32)
		if _, err := rand.Read(tb); err != nil {
			log.Fatalf("generate session token: %v", err)
		}
		token := hex.EncodeToString(tb)
		_, err := pool.Exec(ctx,
			`INSERT INTO sessions (token, user_id, created_at, expires_at)
			 VALUES ($1, $2, $3, $4)`,
			token, uid, time.Now().UTC(), time.Now().Add(72*time.Hour).UTC(),
		)
		if err != nil {
			log.Fatalf("insert session for %s: %v", uid, err)
		}
		fmt.Printf("session token for %s: %s\n", uid, token)
	}

	beritas := []seedBerita{
		{authorIdx: 0, title: "Selamat Datang di JHIC", content: "Ini adalah berita pertama di portal JHIC."},
		{authorIdx: 0, title: "Pembukaan Pendaftaran Anggota Baru", content: "Pendaftaran anggota baru JHIC telah dibuka."},
		{authorIdx: 1, title: "Liputan Kegiatan Jurnalistik", content: "Kegiatan jurnalistik hari ini berjalan dengan lancar."},
		{authorIdx: 1, title: "Wawancara Eksklusif dengan Tokoh Masyarakat", content: "Berikut adalah wawancara eksklusif kami."},
		{authorIdx: 0, title: "Pengumuman Libur Nasional", content: "JHIC libur pada hari libur nasional."},
		{authorIdx: 1, title: "Prestasi: Juara 1 Lomba Jurnalistik", content: "Tim JHIC meraih Juara 1 Lomba Jurnalistik tingkat kota.", isAchievement: true},
	}

	for _, b := range beritas {
		bid := id.New()
		now := time.Now().UTC()
		_, err := pool.Exec(ctx,
			`INSERT INTO berita (id, author_id, title, content, image_url, is_achievement, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, '', $5, $6, $7)
			 ON CONFLICT (id) DO NOTHING`,
			bid, userIDs[b.authorIdx], b.title, b.content, b.isAchievement, now, now,
		)
		if err != nil {
			log.Fatalf("insert berita: %v", err)
		}
		fmt.Printf("berita: %s by %s\n", b.title, users[b.authorIdx].email)
	}

	fmt.Println("\nseed complete")
}

func jurusanClasses(jurusan string) int {
	switch jurusan {
	case "PPLG":
		return 3
	case "AK":
		return 2
	case "HTL":
		return 5
	}
	return 0
}

func slugify(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", "-"))
}

func init() {
	log.SetFlags(0)
}
