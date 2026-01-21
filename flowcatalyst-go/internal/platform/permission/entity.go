package permission

import "time"

// Permission represents a permission
type Permission struct {
	ID          string    `bson:"_id" json:"id"`
	Code        string    `bson:"code" json:"code"`
	Name        string    `bson:"name" json:"name"`
	Description string    `bson:"description,omitempty" json:"description,omitempty"`
	Category    string    `bson:"category,omitempty" json:"category,omitempty"`
	CreatedAt   time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time `bson:"updatedAt" json:"updatedAt"`
}
