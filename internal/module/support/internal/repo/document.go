package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/perfect-panel/server/internal/repository"

	"github.com/perfect-panel/server/internal/module/support/entity/document"
	"github.com/perfect-panel/server/pkg/cache"
	"github.com/perfect-panel/server/pkg/orm"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var cacheDocumentIdPrefix = "cache:document:id:"

var _ repository.DocumentRepo = (*documentRepo)(nil)

type documentRepo struct {
	cache.CachedConn
	table string
}

// NewDocumentRepo builds the module-owned implementation over the shared
// cached connection.
func NewDocumentRepo(conn cache.CachedConn) repository.DocumentRepo {
	return &documentRepo{
		CachedConn: conn,
		table:      "document",
	}
}

//nolint:unused
func (m *documentRepo) batchGetCacheKeys(Documents ...*document.Document) []string {
	var keys []string
	for _, document := range Documents {
		keys = append(keys, m.getCacheKeys(document)...)
	}
	return keys

}

func (m *documentRepo) getCacheKeys(data *document.Document) []string {
	if data == nil {
		return []string{}
	}
	documentIdKey := fmt.Sprintf("%s%v", cacheDocumentIdPrefix, data.Id)
	cacheKeys := []string{
		documentIdKey,
	}
	return cacheKeys
}

func (m *documentRepo) Insert(ctx context.Context, data *document.Document) error {
	err := m.ExecCtx(ctx, func(conn *gorm.DB) error {
		return conn.Create(&data).Error
	}, m.getCacheKeys(data)...)
	return err
}

func (m *documentRepo) FindOne(ctx context.Context, id int64) (*document.Document, error) {
	var resp document.Document
	err := m.QueryNoCacheCtx(ctx, &resp, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&document.Document{}).Where("id = ?", id).First(&resp).Error
	})
	switch {
	case err == nil:
		return &resp, nil
	default:
		return nil, err
	}
}

func (m *documentRepo) Update(ctx context.Context, data *document.Document) error {
	old, err := m.FindOne(ctx, data.Id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	err = m.ExecCtx(ctx, func(conn *gorm.DB) error {
		db := conn
		return db.Save(data).Error
	}, m.getCacheKeys(old)...)
	return err
}

func (m *documentRepo) Delete(ctx context.Context, id int64) error {
	data, err := m.FindOne(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	err = m.ExecCtx(ctx, func(conn *gorm.DB) error {
		db := conn
		return db.Delete(&document.Document{}, id).Error
	}, m.getCacheKeys(data)...)
	return err
}

// QueryDocumentDetail queries the details of a document.
func (m *documentRepo) QueryDocumentDetail(ctx context.Context, id int64) (*document.Document, error) {
	var data document.Document
	err := m.QueryNoCacheCtx(ctx, &data, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&document.Document{}).Preload("Group").Where("id = ?", id).Find(v).Error
	})
	return &data, err
}

// QueryDocumentList queries a list of documents.
func (m *documentRepo) QueryDocumentList(ctx context.Context, page, size int, tag string, search string) (int64, []*document.Document, error) {
	var data []*document.Document
	var total int64
	page, size = repository.NormalizePage(page, size)
	err := m.QueryNoCacheCtx(ctx, &data, func(conn *gorm.DB, v interface{}) error {
		db := conn.Model(&document.Document{})
		if tag != "" {
			db = db.Scopes(orm.CommaSeparatedContains("tags", []string{tag}))
		}
		if search != "" {
			db = db.Scopes(orm.ContainsLike([]string{"title", "content"}, search))
		}
		return db.Count(&total).Offset((page - 1) * size).Limit(size).Find(v).Error
	})
	return total, data, err
}

// GetDocumentListByAll queries a list of documents.
func (m *documentRepo) GetDocumentListByAll(ctx context.Context) (int64, []*document.Document, error) {
	var data []*document.Document
	var total int64
	err := m.QueryNoCacheCtx(ctx, &data, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&document.Document{}).Where(clause.Eq{
			Column: clause.Column{Name: "show"},
			Value:  true,
		}).Count(&total).Find(v).Error
	})
	return total, data, err
}
