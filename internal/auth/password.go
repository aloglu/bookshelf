package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A Password checks what someone typed at sign-in.
type Password interface {
	Match(typed string) bool
}

// Plain is a password given as itself (BOOKSHELF_ADMIN_PASSWORD). It is kept
// as a SHA-256 so comparisons take the same time whatever the input.
func Plain(password string) Password { return plain(sha256.Sum256([]byte(password))) }

type plain [32]byte

func (p plain) Match(typed string) bool {
	t := sha256.Sum256([]byte(typed))
	return subtle.ConstantTimeCompare(t[:], p[:]) == 1
}

// PasswordFile is where `bookshelf set-password` keeps the password, in the
// data folder: a salted PBKDF2-SHA256 hash, never the password itself.
const PasswordFile = "password"

const iterations = 600_000

// MinPassword is the shortest password set-password accepts.
const MinPassword = 8

type stored struct {
	iter       int
	salt, hash []byte
}

func (s stored) Match(typed string) bool {
	h, err := pbkdf2.Key(sha256.New, typed, s.salt, s.iter, len(s.hash))
	return err == nil && subtle.ConstantTimeCompare(h, s.hash) == 1
}

// SavePassword keeps password, hashed, in dir.
func SavePassword(dir, password string) error {
	if len([]rune(password)) < MinPassword {
		return fmt.Errorf("the password must be at least %d characters", MinPassword)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	hash, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("pbkdf2-sha256$%d$%s$%s\n", iterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".password-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(line); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, PasswordFile)) // CreateTemp's file is readable by its owner only
}

// LoadPassword reads the password set-password kept in dir. It returns an
// error satisfying errors.Is(err, os.ErrNotExist) when none was set.
func LoadPassword(dir string) (Password, error) {
	raw, err := os.ReadFile(filepath.Join(dir, PasswordFile))
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimSpace(string(raw)), "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return nil, errors.New("the password file in the data folder isn't one Bookshelf wrote; run bookshelf set-password again")
	}
	iter, err1 := strconv.Atoi(parts[1])
	salt, err2 := base64.RawStdEncoding.DecodeString(parts[2])
	hash, err3 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil || err3 != nil || iter < 1 || len(salt) == 0 || len(hash) == 0 {
		return nil, errors.New("the password file in the data folder is damaged; run bookshelf set-password again")
	}
	return stored{iter: iter, salt: salt, hash: hash}, nil
}
