package upload

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var blogImagePath = regexp.MustCompile(`^/blogs/(?:[0-9]|1[0-5])/(?:[0-9]|1[0-5])/(?:[a-f0-9]{32}|[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})\.(?:jpg|jpeg|png|gif|webp)$`)

type blogImages struct{ root *os.Root }

var errInvalidName = errors.New("invalid image name")
var errNotOwner = errors.New("image owner does not match")

func (s blogImages) save(data []byte, ext string, userID int64) (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	name := fmt.Sprintf("blogs/%d/%d/%s.%s", random[0]&15, random[1]&15, hex.EncodeToString(random[:]), ext)
	if err := s.root.MkdirAll(path.Dir(name), 0750); err != nil {
		return "", err
	}
	if err := s.writeNewFile(name+".owner", []byte(strconv.FormatInt(userID, 10))); err != nil {
		return "", err
	}
	if err := s.writeNewFile(name, data); err != nil {
		_ = s.root.Remove(name + ".owner")
		return "", err
	}
	return "/" + name, nil
}

func (s blogImages) remove(name string, userID int64) error {
	if !blogImagePath.MatchString(name) {
		return errInvalidName
	}
	name = strings.TrimPrefix(name, "/")
	owner, err := s.root.ReadFile(name + ".owner")
	if errors.Is(err, os.ErrNotExist) || (err == nil && string(owner) != strconv.FormatInt(userID, 10)) {
		return errNotOwner
	}
	if err != nil {
		return err
	}
	if err := s.root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := s.root.Remove(name + ".owner"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s blogImages) open(name string) (*os.File, os.FileInfo, error) {
	if !blogImagePath.MatchString(name) {
		return nil, nil, errInvalidName
	}
	file, err := s.root.Open(strings.TrimPrefix(name, "/"))
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, os.ErrNotExist
	}
	return file, info, nil
}

func (s blogImages) writeNewFile(name string, data []byte) error {
	file, err := s.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0640)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		_ = s.root.Remove(name)
		return err
	}
	return nil
}
