package shop

import (
	"context"
	"strings"
	"time"
)

type shopWriteInput struct {
	ID        *int64   `json:"id"`
	Name      *string  `json:"name"`
	TypeID    *int64   `json:"typeId"`
	Images    *string  `json:"images"`
	Area      *string  `json:"area"`
	Address   *string  `json:"address"`
	X         *float64 `json:"x"`
	Y         *float64 `json:"y"`
	AvgPrice  *int64   `json:"avgPrice"`
	Sold      *int     `json:"sold"`
	Comments  *int     `json:"comments"`
	Score     *int     `json:"score"`
	OpenHours *string  `json:"openHours"`
}

type shopWriter interface {
	CreateShop(context.Context, shopWriteInput) (int64, error)
	UpdateShop(context.Context, int64, shopWriteInput) error
}

type shopWriteValues struct {
	columns []string
	args    []any
}

func addShopValue[T any](values *shopWriteValues, column string, value *T) {
	if value != nil {
		values.columns = append(values.columns, column)
		values.args = append(values.args, *value)
	}
}

func (input shopWriteInput) values() shopWriteValues {
	var values shopWriteValues
	addShopValue(&values, "name", input.Name)
	addShopValue(&values, "type_id", input.TypeID)
	addShopValue(&values, "images", input.Images)
	addShopValue(&values, "area", input.Area)
	addShopValue(&values, "address", input.Address)
	addShopValue(&values, "x", input.X)
	addShopValue(&values, "y", input.Y)
	addShopValue(&values, "avg_price", input.AvgPrice)
	addShopValue(&values, "sold", input.Sold)
	addShopValue(&values, "comments", input.Comments)
	addShopValue(&values, "score", input.Score)
	addShopValue(&values, "open_hours", input.OpenHours)
	return values
}

func (r sqlShopReader) CreateShop(ctx context.Context, input shopWriteInput) (int64, error) {
	values := input.values()
	addShopValue(&values, "id", input.ID)
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(values.args)), ", ")
	query := "INSERT INTO tb_shop (" + strings.Join(values.columns, ", ") + ") VALUES (" + placeholders + ")"
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, err := r.db.ExecContext(ctx, query, values.args...)
	if err != nil {
		return 0, err
	}
	if input.ID != nil && *input.ID > 0 {
		return *input.ID, nil
	}
	return result.LastInsertId()
}

func (r sqlShopReader) UpdateShop(ctx context.Context, id int64, input shopWriteInput) error {
	values := input.values()
	if len(values.columns) == 0 {
		return nil
	}
	assignments := make([]string, len(values.columns))
	for i, column := range values.columns {
		assignments[i] = column + " = ?"
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := r.db.ExecContext(ctx, "UPDATE tb_shop SET "+strings.Join(assignments, ", ")+" WHERE id = ?", append(values.args, id)...)
	return err
}
