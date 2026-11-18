package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"time"
)

const (
	loginCodeTTL = 2 * time.Minute
	loginUserTTL = 30 * time.Minute
)

var (
	phonePattern   = regexp.MustCompile(`^1([38][0-9]|4[579]|5[0-35-9]|66|7[0135678]|9[89])[0-9]{8}$`)
	codePattern    = regexp.MustCompile(`^[0-9]{6}$`)
	errCodePending = errors.New("login code already issued")
	errInvalidCode = errors.New("invalid login code")
)

type Summary struct {
	ID       int64   `json:"id"`
	NickName *string `json:"nickName,omitempty"`
	Icon     *string `json:"icon,omitempty"`
}

type userDirectory interface {
	FindOrCreateUser(context.Context, string) (Summary, error)
	FindUser(context.Context, int64) (*Summary, error)
	FindUserInfo(context.Context, int64) (*userInfo, error)
}

type CodeSender interface {
	SendCode(context.Context, string, string) error
}

type sessionRepository interface {
	putCode(context.Context, string, string) (bool, error)
	consumeCode(context.Context, string, string) (bool, error)
	discardCode(context.Context, string, string) error
	saveSession(context.Context, string, Summary) error
	lookupSession(context.Context, string) (*Summary, error)
	dropSession(context.Context, string) error
	setSign(context.Context, string, int64) error
	getSigns(context.Context, string) ([]byte, error)
}

type Service struct {
	sessions sessionRepository
	users    userDirectory
	sender   CodeSender
	now      func() time.Time
}

func (a *Service) issueCode(ctx context.Context, phone string) error {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	created, err := a.sessions.putCode(ctx, phone, code)
	if err != nil {
		return err
	}
	if !created {
		return errCodePending
	}
	if err := a.sender.SendCode(ctx, phone, code); err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = a.sessions.discardCode(cleanup, phone, code)
		return err
	}
	return nil
}

func (a *Service) login(ctx context.Context, phone, code string) (string, error) {
	matched, err := a.sessions.consumeCode(ctx, phone, code)
	if err != nil {
		return "", err
	}
	if !matched {
		return "", errInvalidCode
	}
	user, err := a.users.FindOrCreateUser(ctx, phone)
	if err != nil {
		return "", err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(random[:])
	if err := a.sessions.saveSession(ctx, token, user); err != nil {
		return "", err
	}
	return token, nil
}

func (a *Service) signKey(id int64) (string, int) {
	now := a.now()
	return "sign:" + now.Format("2006:01:") + strconv.FormatInt(id, 10), now.Day()
}

func (a *Service) sign(ctx context.Context, id int64) error {
	key, day := a.signKey(id)
	return a.sessions.setSign(ctx, key, int64(day-1))
}

func (a *Service) signCount(ctx context.Context, id int64) (int, error) {
	key, day := a.signKey(id)
	bitmap, err := a.sessions.getSigns(ctx, key)
	if err != nil {
		return 0, err
	}
	count := 0
	for offset := day - 1; offset >= 0; offset-- {
		if offset/8 >= len(bitmap) || bitmap[offset/8]&(0x80>>uint(offset%8)) == 0 {
			break
		}
		count++
	}
	return count, nil
}

func (a *Service) lookupSession(ctx context.Context, token string) (*Summary, error) {
	return a.sessions.lookupSession(ctx, token)
}

func (a *Service) logout(ctx context.Context, token string) error {
	return a.sessions.dropSession(ctx, token)
}

func (a *Service) account(ctx context.Context, id int64) (*Summary, error) {
	return a.users.FindUser(ctx, id)
}

func (a *Service) profile(ctx context.Context, id int64) (*userInfo, error) {
	return a.users.FindUserInfo(ctx, id)
}
