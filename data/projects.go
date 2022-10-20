package data

import (
	"gorm.io/gorm"
)

func NewProjectsDAO(db *gorm.DB) *ProjectsDAO {
	return &ProjectsDAO{db}
}

type ProjectUpdate struct {
	Name string `json:"value"`
}

type ProjectsDAO struct {
	db *gorm.DB
}

func (d *ProjectsDAO) GetOne(id int) (Project, error) {
	project := Project{}
	err := d.db.Find(&project, id).Error
	return project, err
}

func (d *ProjectsDAO) GetAll() ([]Project, error) {
	projects := make([]Project, 0)
	err := d.db.Find(&projects).Error
	return projects, err
}

func (d *ProjectsDAO) Add(update *ProjectUpdate) (int, error) {
	project := Project{}
	update.fillModel(&project)

	err := d.db.Save(&project).Error
	return project.ID, err
}

func (d *ProjectsDAO) Update(id int, update *ProjectUpdate) error {
	project, err := d.GetOne(id)
	if err != nil {
		return err
	}

	update.fillModel(&project)
	err = d.db.Save(&project).Error
	return err
}

func (d *ProjectsDAO) Delete(id int) error {
	tasks := make([]Task, 0)
	err := d.db.Select("id").Where("project = ?", id).Find(&tasks).Error
	if err != nil {
		return err
	}

	ids := make([]int, len(tasks))
	for i := range tasks {
		ids[i] = tasks[i].ID
	}

	err = d.db.Exec("DELETE FROM assigned_users WHERE task_id IN ?", ids).Error
	if err != nil {
		return err
	}
	err = d.db.Where("id IN ?", ids).Delete(&Task{}).Error
	if err != nil {
		return err
	}
	err = d.db.Delete(&Project{}, id).Error

	return err
}

func (d *ProjectUpdate) fillModel(ev *Project) {
	if ev != nil {
		ev.Name = d.Name
	}
}
