package user

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

type userInfo struct {
	UserID    int64   `json:"userId"`
	City      *string `json:"city,omitempty"`
	Introduce *string `json:"introduce,omitempty"`
	Fans      *int    `json:"fans,omitempty"`
	Followee  *int    `json:"followee,omitempty"`
	Gender    *bool   `json:"gender,omitempty"`
	Birthday  *string `json:"birthday,omitempty"`
	Credits   *int    `json:"credits,omitempty"`
	Level     *bool   `json:"level,omitempty"`
}

type sqlUsers struct{ db *sql.DB }

func scanUser(row *sql.Row) (*Summary, error) {
	var user Summary
	err := row.Scan(&user.ID, &user.NickName, &user.Icon)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r sqlUsers) FindOrCreateUser(ctx context.Context, phone string) (Summary, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	const lookup = "SELECT id, nick_name, icon FROM tb_user WHERE phone = ?"
	user, err := scanUser(r.db.QueryRowContext(ctx, lookup, phone))
	if err != nil {
		return Summary{}, err
	}
	if user != nil {
		return *user, nil
	}
	var random [5]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Summary{}, err
	}
	_, err = r.db.ExecContext(ctx, "INSERT INTO tb_user (phone, nick_name) VALUES (?, ?)", phone, "user_"+hex.EncodeToString(random[:]))
	var mysqlErr *mysql.MySQLError
	if err != nil && (!errors.As(err, &mysqlErr) || mysqlErr.Number != 1062) {
		return Summary{}, err
	}
	user, err = scanUser(r.db.QueryRowContext(ctx, lookup, phone))
	if err != nil {
		return Summary{}, err
	}
	if user == nil {
		return Summary{}, errors.New("user missing after creation")
	}
	return *user, nil
}

func (r sqlUsers) FindUser(ctx context.Context, id int64) (*Summary, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return scanUser(r.db.QueryRowContext(ctx, "SELECT id, nick_name, icon FROM tb_user WHERE id = ?", id))
}

func (r sqlUsers) FindUserInfo(ctx context.Context, id int64) (*userInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var info userInfo
	var birthday *time.Time
	var gender, level *int
	err := r.db.QueryRowContext(ctx, "SELECT user_id, city, introduce, fans, followee, gender, birthday, credits, level FROM tb_user_info WHERE user_id = ?", id).
		Scan(&info.UserID, &info.City, &info.Introduce, &info.Fans, &info.Followee, &gender, &birthday, &info.Credits, &level)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if birthday != nil {
		info.Birthday = new(birthday.Format("2006-01-02"))
	}
	if gender != nil {
		info.Gender = new(*gender != 0)
	}
	if level != nil {
		info.Level = new(*level != 0)
	}
	return &info, nil
}
