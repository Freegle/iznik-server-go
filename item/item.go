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

	// Return nil when no item record exists (e.g. TrashNothing messages).
	// PHP omits the item key entirely when there's no record.
	if item.ID == 0 {
		return nil
	}

	return &item
}
