package data

import (
	"encoding/json"
	"os"
)

func dataUp(d *DAO) (err error) {
	tx := d.GetDB().Begin()
	defer func() {
		if err == nil {
			tx.Commit()
		} else {
			tx.Rollback()
			panic(err)
		}
	}()

	projects := make([]Project, 0)
	err = parseDemodata(&projects, "./demodata/projects.json")
	if err != nil {
		return
	}
	err = tx.Create(&projects).Error
	if err != nil {
		return
	}

	users := make([]User, 0)
	err = parseDemodata(&users, "./demodata/users.json")
	if err != nil {
		return
	}
	err = tx.Create(&users).Error
	if err != nil {
		return
	}

	tasks := make([]Task, 0)
	err = parseDemodata(&tasks, "./demodata/tasks.json")
	if err != nil {
		return
	}
	indexPull := make(map[int]int)
	for i := range tasks {
		if len(tasks[i].AssignedUsersIDs) > 0 {
			users := make([]User, 0)
			err = tx.Where("id IN(?)", tasks[i].AssignedUsersIDs).Find(&users).Error
			if err != nil {
				return
			}
			tasks[i].AssignedUsers = users
		}
		tasks[i].Index = indexPull[tasks[i].ProjectID]
		indexPull[tasks[i].ProjectID]++
	}
	err = tx.Create(&tasks).Error

	return
}

func parseDemodata(dest interface{}, path string) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	err = json.Unmarshal(bytes, &dest)

	return err
}
