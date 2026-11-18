package localtime

import (
	"encoding/json"
	"fmt"
	"time"
)

type Time time.Time

func (t *Time) Scan(value any) error {
	valueTime, ok := value.(time.Time)
	if !ok {
		return fmt.Errorf("scan local timestamp: expected time.Time, got %T", value)
	}
	*t = Time(valueTime)
	return nil
}

func (t Time) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(t).Format("2006-01-02T15:04:05.999999999"))
}

func (t *Time) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	parsed, err := time.Parse("2006-01-02T15:04:05", value)
	if err != nil {
		return err
	}
	*t = Time(parsed)
	return nil
}
