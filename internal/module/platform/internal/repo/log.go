package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/orm"
	"github.com/perfect-panel/server/pkg/requestmeta"
	"gorm.io/gorm"
)

var _ repository.LogRepo = (*logRepo)(nil)

type logRepo struct {
	*gorm.DB
}

// NewLogRepo builds the module-owned implementation.
func NewLogRepo(db *gorm.DB) repository.LogRepo {
	return &logRepo{
		DB: db,
	}
}

func (m *logRepo) Insert(ctx context.Context, data *log.SystemLog) error {
	attachRequestMetadata(ctx, data)
	return m.WithContext(ctx).Create(data).Error
}

func (m *logRepo) InsertBatch(ctx context.Context, data []*log.SystemLog, batchSize int) error {
	if len(data) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 1000
	}
	for _, item := range data {
		attachRequestMetadata(ctx, item)
	}
	return m.WithContext(ctx).CreateInBatches(data, batchSize).Error
}

func (m *logRepo) FindOne(ctx context.Context, id int64) (*log.SystemLog, error) {
	var data log.SystemLog
	err := m.WithContext(ctx).Where("id = ?", id).First(&data).Error
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func (m *logRepo) Update(ctx context.Context, data *log.SystemLog) error {
	attachRequestMetadata(ctx, data)
	return m.WithContext(ctx).Where("id = ?", data.Id).Save(data).Error
}

// attachRequestMetadata adds request-origin metadata to every JSON audit row
// written inside an HTTP request. Existing type-specific fields win so login,
// registration and subscription logs keep their public JSON contracts. The
// operation is deliberately best-effort: malformed legacy content must retain
// its existing database error behavior rather than making metadata enrichment
// a new source of failed financial transactions.
func attachRequestMetadata(ctx context.Context, data *log.SystemLog) {
	if data == nil || data.Content == "" {
		return
	}
	metadata, ok := requestmeta.From(ctx)
	if !ok {
		return
	}
	var content map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data.Content), &content); err != nil || content == nil {
		return
	}
	changed := false
	if metadata.ClientIP != "" {
		ipKey := "client_ip"
		switch log.Type(data.Type) {
		case log.TypeLogin:
			ipKey = "login_ip"
		case log.TypeRegister:
			ipKey = "register_ip"
		}
		changed = setStringIfEmpty(content, ipKey, metadata.ClientIP) || changed
	}
	if metadata.UserAgent != "" {
		changed = setStringIfEmpty(content, "user_agent", metadata.UserAgent) || changed
	}
	if metadata.ActorID > 0 {
		if _, exists := content["actor_id"]; !exists {
			content["actor_id"], _ = json.Marshal(metadata.ActorID)
			changed = true
		}
	}
	for key, value := range map[string]string{
		"ip_country_code":    metadata.IPCountryCode,
		"ip_country":         metadata.IPCountry,
		"ip_region":          metadata.IPRegion,
		"ip_city":            metadata.IPCity,
		"ip_as_organization": metadata.IPASOrganization,
	} {
		if value != "" {
			changed = setStringIfEmpty(content, key, value) || changed
		}
	}
	if metadata.IPASN > 0 {
		if _, exists := content["ip_asn"]; !exists {
			content["ip_asn"], _ = json.Marshal(metadata.IPASN)
			changed = true
		}
	}
	if !changed {
		return
	}
	if encoded, err := json.Marshal(content); err == nil {
		data.Content = string(encoded)
	}
}

func setStringIfEmpty(content map[string]json.RawMessage, key, value string) bool {
	if raw, exists := content[key]; exists {
		var current string
		if json.Unmarshal(raw, &current) == nil && current != "" {
			return false
		}
	}
	content[key], _ = json.Marshal(value)
	return true
}

func (m *logRepo) Delete(ctx context.Context, id int64) error {
	return m.WithContext(ctx).Where("id = ?", id).Delete(&log.SystemLog{}).Error
}

// FilterSystemLog filter system logs with pagination
func (m *logRepo) FilterSystemLog(ctx context.Context, filter *log.FilterParams) ([]*log.SystemLog, int64, error) {
	tx := m.WithContext(ctx).Model(&log.SystemLog{}).Order("id DESC")
	if filter == nil {
		filter = &log.FilterParams{
			Page: 1,
			Size: repository.DefaultPageSize,
		}
	}

	filter.Page, filter.Size = repository.NormalizePage(filter.Page, filter.Size)

	if filter.Type != 0 {
		tx = tx.Where("type = ?", filter.Type)
	}

	if filter.Data != "" {
		tx = tx.Where("date = ?", filter.Data)
	}

	if filter.ObjectID != 0 {
		tx = tx.Where("object_id = ?", filter.ObjectID)
	}
	if filter.Search != "" {
		tx = tx.Scopes(orm.ContainsLike([]string{"content"}, filter.Search))
	}

	var total int64
	var logs []*log.SystemLog
	if !filter.SkipCount {
		if err := tx.Count(&total).Error; err != nil {
			return nil, 0, err
		}
	}
	err := tx.Limit(filter.Size).Offset((filter.Page - 1) * filter.Size).Find(&logs).Error
	return logs, total, err
}

// FindFirstByDateType find first system log by date and type
func (m *logRepo) FindFirstByDateType(ctx context.Context, date string, typ uint8) (*log.SystemLog, error) {
	var data log.SystemLog
	err := m.WithContext(ctx).Model(&log.SystemLog{}).Where("date = ? AND type = ?", date, typ).First(&data).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &data, nil
}

// FindByDatesType find system logs by dates and type
func (m *logRepo) FindByDatesType(ctx context.Context, dates []string, typ uint8) ([]*log.SystemLog, error) {
	var data []*log.SystemLog
	if len(dates) == 0 {
		return data, nil
	}
	err := m.WithContext(ctx).Model(&log.SystemLog{}).Where("date IN ? AND type = ?", dates, typ).Find(&data).Error
	return data, err
}

// DeleteBefore deletes system logs whose date is before the given end date.
func (m *logRepo) DeleteBefore(ctx context.Context, end time.Time) error {
	return m.WithContext(ctx).
		Where("date < ? AND type IN ?", end.Format(time.DateOnly), log.ExpirableTypes()).
		Delete(&log.SystemLog{}).Error
}

func (m *logRepo) DeleteBeforeBatch(ctx context.Context, end time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	var ids []int64
	if err := m.WithContext(ctx).Model(&log.SystemLog{}).
		Select("id").
		Where("date < ? AND type IN ?", end.Format(time.DateOnly), log.ExpirableTypes()).
		Order("id ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := m.WithContext(ctx).Where("id IN ?", ids).Delete(&log.SystemLog{})
	return result.RowsAffected, result.Error
}

// SumAmountByTypeAndObjectID returns the sum of the "amount" field extracted from JSON content
// for all system logs matching the given type and object ID.
func (m *logRepo) SumAmountByTypeAndObjectID(ctx context.Context, typ uint8, objectID int64) (int64, error) {
	jsonExtract := jsonAmountExpr(m.DB)
	var sum int64
	err := m.WithContext(ctx).
		Model(&log.SystemLog{}).
		Select(fmt.Sprintf("COALESCE(SUM(%s), 0)", jsonExtract)).
		Where("type = ? AND object_id = ?", typ, objectID).
		Scan(&sum).Error
	return sum, err
}

func jsonAmountExpr(db *gorm.DB) string {
	if db != nil && db.Dialector.Name() == orm.DriverPostgres {
		return "(content::json->>'amount')::bigint"
	}
	return "CAST(JSON_EXTRACT(content, '$.amount') AS SIGNED)"
}
