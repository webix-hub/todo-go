package data

import (
	"web-widgets/todo-go/common"

	"gorm.io/gorm"
)

type TaskUpdate struct {
	TaskProps
	AfterID int    `json:"after"`
	Batch   []Task `json:"batch"`
}

type ShiftInfo struct {
	ParentID int `json:"parent"`
}

type MoveInfo struct {
	ID        int             `json:"id"`
	ParentID  int             `json:"parent"`
	AfterID   int             `json:"after"`
	ProjectID common.FuzzyInt `json:"project"`
	Operation string          `json:"operation"`
	Reverse   bool            `json:"reverse"`
	Batch     []int           `json:"batch,omitempty"`
}

type PasteInfo struct {
	AfterID   int    `json:"after"`
	ParentID  int    `json:"parent"`
	ProjectID int    `json:"project"`
	Batch     []Task `json:"batch"`
}

type SortInfo struct {
	By        string `json:"by"`
	Direction string `json:"dir"`
}

type TasksDAO struct {
	db *gorm.DB
}

func NewTasksDAO(db *gorm.DB) *TasksDAO {
	return &TasksDAO{db}
}

func (d *TasksDAO) GetOne(id int) (*Task, error) {
	task := Task{}
	err := d.db.Find(&task, id).Error
	return &task, err
}

func (d *TasksDAO) GetAll() ([]Task, error) {
	tasks := make([]Task, 0)
	err := d.db.
		Order("project, `index` asc").
		Preload("AssignedUsers").
		Find(&tasks).Error

	for i, c := range tasks {
		tasks[i].AssignedUsersIDs = getIDs(c.AssignedUsers)
	}
	return tasks, err
}

func (d *TasksDAO) GetFromProject(id int) ([]Task, error) {
	var err error
	tasks := make([]Task, 0)
	err = d.db.
		Where("project = ?", id).
		Order("`index` asc").
		Preload("AssignedUsers").
		Find(&tasks).Error

	for i, c := range tasks {
		tasks[i].AssignedUsersIDs = getIDs(c.AssignedUsers)
	}
	return tasks, err
}

func (d *TasksDAO) Add(update *TaskUpdate) (int, error) {
	var err error
	var index int
	if update.AfterID != 0 {
		var afterTask *Task
		afterTask, err = d.GetOne(update.AfterID)
		if err != nil {
			return 0, err
		}

		if update.AfterID == update.ParentID {
			// add sub-task
			index, err = d.getMaxIndex(afterTask.ProjectID, afterTask.ID)
		} else {
			// add task below
			index = afterTask.Index
			var direction int
			direction, err = d.updateIndex(afterTask.ProjectID, afterTask.ParentID, index, 1)
			if direction > 0 {
				index++
			}
		}
	} else {
		// add task at the start of the tree
		index, err = d.getMinIndex(update.ProjectID, 0)
	}
	if err != nil {
		return 0, err
	}
	task := update.toModel()
	task.Index = index
	err = d.db.Create(&task).Error

	return int(task.ID), err
}

func (d *TasksDAO) Update(id int, update *TaskUpdate) (err error) {
	tx := d.openTX()
	defer d.closeTX(tx, err)

	if len(update.Batch) > 0 {
		for _, v := range update.Batch {
			t := Task{}
			err = d.db.Find(&t, v.ID).Error
			if err != nil {
				return
			}
			err = tx.Save(&v).Error
			if err != nil {
				return
			}
		}
	}

	task, err := d.GetOne(id)
	if err != nil {
		return
	}

	task.Text = update.Text
	task.Checked = update.Checked
	task.DueDate = update.DueDate

	err = tx.Model(&task).Association("AssignedUsers").Clear()
	if err != nil {
		return
	}
	if len(update.AssignedUsersIDs) > 0 {
		users := make([]User, 0)
		err := d.db.Where("id IN(?)", update.AssignedUsersIDs).Find(&users).Error
		if err != nil {
			return err
		}
		task.AssignedUsers = users
	}
	err = tx.Save(&task).Error
	if err != nil {
		return
	}
	return
}

func (d *TasksDAO) Delete(id int) error {
	root, err := d.GetOne(id)
	if err != nil {
		return err
	}
	ids, err := d.getChildrenIDs(root.ProjectID, id)
	if err != nil {
		return err
	}
	ids = append(ids, id)
	err = d.db.Exec("DELETE FROM assigned_users WHERE task_id IN ?", ids).Error
	if err == nil {
		err = d.db.Where("id IN ?", ids).Delete(&Task{}).Error
	}

	return err
}

func (d *TasksDAO) MoveToProject(id int, info *MoveInfo) (err error) {
	if id != 0 {
		info.Batch = []int{id}
	}

	tx := d.openTX()
	defer d.closeTX(tx, err)

	index, err := d.getMaxIndex(int(info.ProjectID), 0)
	if err != nil {
		return err
	}
	for _, id := range info.Batch {
		var task Task
		err := tx.Find(&task, id).Error
		if err != nil {
			return err
		}

		oldProject := task.ProjectID

		task.ParentID = 0
		task.ProjectID = int(info.ProjectID)
		task.Index = index
		index++

		err = tx.Save(task).Error
		if err != nil {
			return err
		}

		taskChildren, err := d.getChildrenIDs(oldProject, id)
		if err != nil {
			return err
		}

		if len(taskChildren) > 0 {
			err = tx.Model(&Task{}).Where("id IN ?", taskChildren).Update("project", info.ProjectID).Error
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *TasksDAO) Indent(id int, info *ShiftInfo) error {
	task, err := d.GetOne(id)
	if err != nil {
		return err
	}

	// check if parent task exists
	_, err = d.GetOne(info.ParentID)
	if err != nil {
		return err
	}
	index, err := d.getMaxIndex(task.ProjectID, info.ParentID)

	if err == nil {
		task.Index = index
		task.ParentID = info.ParentID
		err = d.db.Save(task).Error
	}

	return err
}

func (d *TasksDAO) Unindent(id int, info *ShiftInfo) error {
	task, err := d.GetOne(id)
	if err != nil {
		return err
	}

	parentTask, err := d.GetOne(task.ParentID)
	if err != nil {
		return err
	}
	nextTask, err := d.getNextTaskByIndex(task.ProjectID, parentTask.ParentID, parentTask.Index)
	if err != nil {
		return err
	}
	var index int
	if nextTask == nil {
		index = parentTask.Index + 1
	} else {
		index = nextTask.Index
		var dir int
		dir, err = d.updateIndex(task.ProjectID, parentTask.ParentID, index-1, 1)
		if dir < 0 {
			index--
		}
	}

	if err == nil {
		task.Index = index
		task.ParentID = info.ParentID
		err = d.db.Save(task).Error
	}

	return err
}

func (d *TasksDAO) Paste(info *PasteInfo) (idPull map[int]int, err error) {
	if info == nil || len(info.Batch) == 0 {
		return nil, nil
	}

	tx := d.openTX()
	defer d.closeTX(tx, err)

	afterTask, err := d.GetOne(info.AfterID)
	if err != nil {
		return
	}

	// divide into independent parents and cloneable childs
	roots := make([]Task, 0)
	childs := make([]Task, 0)
	offset := 0
	for _, t := range info.Batch {
		isChild := false
		for _, t2 := range info.Batch {
			if t2.ID == t.ParentID {
				isChild = true
				childs = append(childs, t)
				break
			}
		}
		if !isChild {
			roots = append(roots, t)
			offset++
		}
	}

	// create space for clonable tasks
	index := afterTask.Index
	dir, err := d.updateIndexTX(afterTask.ProjectID, afterTask.ParentID, afterTask.Index, offset, tx)
	if err != nil {
		return nil, err
	}
	if dir > 0 {
		index++
	} else {
		index -= offset - 1
	}

	// insert roots
	idPull = make(map[int]int)
	for _, t := range roots {
		task := t.toModel()
		task.ParentID = info.ParentID
		task.Index = index
		index++

		err := tx.Create(&task).Error
		if err != nil {
			return nil, err
		}
		idPull[t.ID] = task.ID
	}

	// insert childs
	indexPull := make(map[int]int)
	for _, t := range childs {
		task := t.toModel()
		task.ParentID = idPull[t.ParentID]
		task.Index = indexPull[t.ParentID]
		indexPull[t.ParentID]++

		err := tx.Create(&task).Error
		if err != nil {
			return nil, err
		}
		idPull[t.ID] = task.ID
	}

	return idPull, nil
}

// helpres

func (d TaskProps) toModel() *Task {
	return &Task{
		TaskProps: d,
	}
}

func (d *TasksDAO) openTX() *gorm.DB {
	return d.db.Begin()
}

func (d *TasksDAO) closeTX(tx *gorm.DB, err error) {
	if err == nil {
		tx.Commit()
	} else {
		tx.Rollback()
	}
}

func (d *TasksDAO) getMinIndex(projectID, parentID int) (int, error) {
	task := Task{}
	err := d.db.
		Where("project = ? AND parent = ?", projectID, parentID).
		Order("`index` ASC").
		Take(&task).Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	return task.Index - 1, err
}

func (d *TasksDAO) getMaxIndex(projectID, parentID int) (int, error) {
	task := Task{}
	err := d.db.
		Where("project = ? AND parent = ?", projectID, parentID).
		Order("`index` DESC").
		Take(&task).Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	return task.Index + 1, err
}

func (d *TasksDAO) getNextTaskByIndex(projectID, parentID, index int) (*Task, error) {
	task := Task{}
	err := d.db.
		Where("project = ? AND parent = ? AND `index` > ?", projectID, parentID, index).
		Order("`index` ASC").
		Take(&task).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}

	return &task, err
}

func (d *TasksDAO) getMinDistance(projectID, parentID, index int) (int, error) {
	var toEnd, toStart int64
	err := d.db.Model(&Task{}).
		Where("project = ? AND parent = ? AND `index` < ?", projectID, parentID, index+1).
		Count(&toStart).Error
	if err != nil {
		return 0, err
	}

	if toStart == 0 {
		return -1, nil
	}

	err = d.db.Model(&Task{}).
		Where("project = ? AND parent = ? AND `index` > ?", projectID, parentID, index-1).
		Count(&toEnd).Error
	if err != nil {
		return 0, err
	}

	if toEnd > toStart {
		return -1, nil
	}
	return 1, nil
}

func (d *TasksDAO) updateIndex(projectID, parentID, from, offset int) (dir int, err error) {
	return d.updateIndexTX(projectID, parentID, from, offset, d.db)
}

func (d *TasksDAO) updateIndexTX(projectID, parentID, from, offset int, tx *gorm.DB) (dir int, err error) {
	direction, err := d.getMinDistance(projectID, parentID, from)
	if err != nil {
		return 0, err
	}

	if direction < 0 {
		// update index to the start where 'index' <= 'from'
		err = tx.Model(&Task{}).
			Where("project = ? AND parent = ? AND `index` < ?", projectID, parentID, from+1).
			Update("index", gorm.Expr("`index` - ?", offset)).Error
	} else {
		// update index to the end where 'index' > 'from'
		err = tx.Model(&Task{}).
			Where("project = ? AND parent = ? AND `index` > ?", projectID, parentID, from).
			Update("index", gorm.Expr("`index` + ?", offset)).Error
	}

	return direction, err
}

func (d *TasksDAO) getChildrenIDs(projectID, taskID int) ([]int, error) {
	arr, err := d.GetFromProject(projectID)
	if err != nil {
		return nil, err
	}

	return findChildren(arr, taskID), nil
}

func findChildren(arr []Task, id int) []int {
	if i := hasChild(arr, id); i == -1 {
		return []int{}
	}

	var storage []int
	for i := range arr {
		if arr[i].ParentID == id {
			storage = append(storage, arr[i].ID)
			storage = append(storage, findChildren(arr, arr[i].ID)...)
		}
	}
	return storage
}

func hasChild(arr []Task, id int) int {
	for i := range arr {
		if arr[i].ParentID == id {
			return i
		}
		i++
	}
	return -1
}

func getIDs(users []User) []int {
	ids := make([]int, len(users))
	for i, card := range users {
		ids[i] = card.ID
	}
	return ids
}
