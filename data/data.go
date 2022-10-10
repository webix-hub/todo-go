package data

import "time"

type TaskProps struct {
	Text      string     `json:"text"`
	Checked   bool       `json:"checked"`
	DueDate   *time.Time `json:"due_date,omitempty"`
	ParentID  int        `json:"parent" gorm:"column:parent"`
	ProjectID int        `json:"project,omitempty" gorm:"column:project"`

	AssignedUsers    []User `gorm:"many2many:assigned_users" json:"-"`
	AssignedUsersIDs []int  `gorm:"-" json:"assigned"`
}

type Task struct {
	TaskProps
	ID    int `json:"id"`
	Index int `json:"-"`
}

type Project struct {
	ID   int    `json:"id"`
	Name string `json:"value" gorm:"column:value"`
}

type User struct {
	ID     int    `json:"id"`
	Name   string `json:"value" gorm:"column:value"`
	Avatar string `json:"path" gorm:"column:path"`

	AssignedCards []Task `gorm:"many2many:assigned_users" json:"-"`
}
