package item

import "github.com/freegle/iznik-server-go/database"

type Item struct {
	ID   uint64 `json:"id" gorm:"primary_key"`
	Name string `json:"name"`
}

func FetchForMessage(msgid uint64) *Item {
	var item Item

	db := database.DBConn

	db.Raw("SELECT items.id, items.name FROM items INNER JOIN messages_items ON items.id = messages_items.itemid WHERE msgid = ?", msgid).Scan(&item)

	return &item
}
