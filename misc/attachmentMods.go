package misc

// We need to define the mods that we support, so that we can decode the JSON we get back from the database, and
// then re-encode it when we send it back.
type AttachmentMods struct {
	format string `json:"format"`
	rotate int    `json:"rotate"`
}
