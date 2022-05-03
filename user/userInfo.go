package user

import (
	"github.com/gofiber/fiber/v2"
	"time"
)

type UserInfo struct {
	Replies    uint64    `json: "replies"`
	Taken      uint64    `json: "taken"`
	Reneged    uint64    `json: "reneged"`
	Collected  uint64    `json: "collected"`
	Openage    uint64    `json: "openage"`
	Lastaccess time.Time `json: "lastaccess"`
}

const OPEN_AGE = 90

func GetUserUinfo(c *fiber.Ctx, id uint64) UserInfo {
	//db := database.DBConn
	//TODO Actually fetch info.

	var info UserInfo

	info.Replies = 0
	info.Taken = 0
	info.Reneged = 0
	info.Collected = 0
	info.Openage = OPEN_AGE

	return info
}

//public function getInfos(&$users, $grace = 30) {
//$uids = array_filter(array_column($users, 'id'));
//
//$start = date('Y-m-d', strtotime(User::OPEN_AGE . " days ago"));
//$days90 = date("Y-m-d", strtotime("90 days ago"));
//$userq = "userid IN (" . implode(',', $uids) . ")";
//
//foreach ($uids as $uid) {
//$users[$uid]['info']['replies'] = 0;
//$users[$uid]['info']['taken'] = 0;
//$users[$uid]['info']['reneged'] = 0;
//$users[$uid]['info']['collected'] = 0;
//$users[$uid]['info']['openage'] = User::OPEN_AGE;
//}
//
//// We can combine some queries into a single one.  This is better for performance because it saves on
//// the round trip (seriously, I've measured it, and it's worth doing).
////
//// No need to check on the chat room type as we can only get messages of type Interested in a User2User chat.
//$tq = Session::modtools() ? ", t6.*, t7.*" : '';
//$sql = "SELECT t0.id AS theuserid, t0.lastaccess AS lastaccess, t1.*, t3.*, t4.*, t5.* $tq FROM
//(SELECT id, lastaccess FROM users WHERE id in (" . implode(',', $uids) . ")) t0 LEFT JOIN
//(SELECT COUNT(DISTINCT refmsgid) AS replycount, userid FROM chat_messages WHERE $userq AND date > ? AND refmsgid IS NOT NULL AND type = ?) t1 ON t1.userid = t0.id LEFT JOIN";
//
//if (Session::modtools()) {
//$sql .= "(SELECT COUNT(DISTINCT refmsgid) AS replycountoffer, userid FROM chat_messages INNER JOIN messages ON messages.id = chat_messages.refmsgid WHERE $userq AND chat_messages.date > '$start' AND refmsgid IS NOT NULL AND chat_messages.type = '" . ChatMessage::TYPE_INTERESTED . "' AND messages.type = '" . Message::TYPE_OFFER . "') t6 ON t6.userid = t0.id LEFT JOIN ";
//$sql .= "(SELECT COUNT(DISTINCT refmsgid) AS replycountwanted, userid FROM chat_messages INNER JOIN messages ON messages.id = chat_messages.refmsgid WHERE $userq AND chat_messages.date > '$start' AND refmsgid IS NOT NULL AND chat_messages.type = '" . ChatMessage::TYPE_INTERESTED . "' AND messages.type = '" . Message::TYPE_WANTED . "') t7 ON t7.userid = t0.id LEFT JOIN ";
//}
//
//$sql .= "(SELECT COUNT(DISTINCT(messages_reneged.msgid)) AS reneged, userid FROM messages_reneged WHERE $userq AND timestamp > ?) t3 ON t3.userid = t0.id LEFT JOIN
//(SELECT COUNT(DISTINCT messages_by.msgid) AS collected, messages_by.userid FROM messages_by INNER JOIN messages ON messages.id = messages_by.msgid INNER JOIN chat_messages ON chat_messages.refmsgid = messages.id AND messages.type = ? AND chat_messages.type = ? INNER JOIN messages_groups ON messages_groups.msgid = messages.id WHERE chat_messages.$userq AND messages_by.$userq AND messages_by.userid != messages.fromuser AND messages_groups.arrival >= '$days90') t4 ON t4.userid = t0.id LEFT JOIN
//(SELECT timestamp AS abouttime, text AS abouttext, userid FROM users_aboutme WHERE $userq ORDER BY timestamp DESC LIMIT 1) t5 ON t5.userid = t0.id
//;";
//$counts = $this->dbhr->preQuery($sql, [
//$start,
//ChatMessage::TYPE_INTERESTED,
//$start,
//Message::TYPE_OFFER,
//ChatMessa	ge::TYPE_INTERESTED
//]);
//
//foreach ($users as $uid => $user) {
//foreach ($counts as $count) {
//if ($count['theuserid'] == $users[$uid]['id']) {
//$users[$uid]['info']['replies'] = $count['replycount'] ? $count['replycount'] : 0;
//
//
//$users[$uid]['info']['reneged'] = $count['reneged'] ? $count['reneged'] : 0;
//$users[$uid]['info']['collected'] = $count['collected'] ? $count['collected'] : 0;
//$users[$uid]['info']['lastaccess'] = $count['lastaccess'] ? Utils::ISODate($count['lastaccess']) : NULL;
//$users[$uid]['info']['count'] = $count;
//
//if (Utils::pres('abouttime', $count)) {
//$users[$uid]['info']['aboutme'] = [
//'timestamp' => Utils::ISODate($count['abouttime']),
//'text' => $count['abouttext']
//];
//}
//}
//}
//}
//
//$sql = "SELECT messages.fromuser AS userid, COUNT(*) AS count, messages.type, messages_outcomes.outcome FROM messages LEFT JOIN messages_outcomes ON messages_outcomes.msgid = messages.id INNER JOIN messages_groups ON messages_groups.msgid = messages.id WHERE fromuser IN (" . implode(',', $uids) . ") AND messages.arrival > ? AND collection = ? AND messages_groups.deleted = 0 GROUP BY messages.fromuser, messages.type, messages_outcomes.outcome;";
//$counts = $this->dbhr->preQuery($sql, [
//$start,
//MessageCollection::APPROVED
//]);
//
//foreach ($users as $uid => $user) {
//$users[$uid]['info']['offers'] = 0;
//$users[$uid]['info']['wanteds'] = 0;
//$users[$uid]['info']['openoffers'] = 0;
//$users[$uid]['info']['openwanteds'] = 0;
//$users[$uid]['info']['expectedreply'] = 0;
//
//foreach ($counts as $count) {
//if ($count['userid'] == $users[$uid]['id']) {
//if ($count['type'] == Message::TYPE_OFFER) {
//$users[$uid]['info']['offers'] += $count['count'];
//
//if (!Utils::pres('outcome', $count)) {
//$users[$uid]['info']['openoffers'] += $count['count'];
//}
//} else if ($count['type'] == Message::TYPE_WANTED) {
//$users[$uid]['info']['wanteds'] += $count['count'];
//
//if (!Utils::pres('outcome', $count)) {
//$users[$uid]['info']['openwanteds'] += $count['count'];
//}
//}
//}
//}
//}
//
//# Distance away.
//$me = Session::whoAmI($this->dbhr, $this->dbhm);
//
//if ($me) {
//list ($mylat, $mylng, $myloc) = $me->getLatLng();
//
//if ($myloc !== NULL) {
//$latlngs = $this->getLatLngs($users);
//
//foreach ($latlngs as $userid => $latlng) {
//$users[$userid]['info']['milesaway'] = $this->getDistanceBetween($mylat, $mylng, $latlng['lat'], $latlng['lng']);
//}
//}
//
//$this->getPublicLocations($users);
//}
//
//$r = new ChatRoom($this->dbhr, $this->dbhm);
//$replytimes = $r->replyTimes($uids);
//
//foreach ($replytimes as $uid => $replytime) {
//$users[$uid]['info']['replytime'] = $replytime;
//}
//
//$nudges = $r->nudgeCounts($uids);
//
//foreach ($nudges as $uid => $nudgecount) {
//$users[$uid]['info']['nudges'] = $nudgecount;
//}
//
//$ratings = $this->getRatings($uids);
//
//foreach ($ratings as $uid => $rating) {
//$users[$uid]['info']['ratings'] = $rating;
//}
//
//$replies = $this->getExpectedReplies($uids, ChatRoom::ACTIVELIM, $grace);
//
//foreach ($replies as $reply) {
//if ($reply['expectee']) {
//$users[$reply['expectee']]['info']['expectedreply'] = $reply['count'];
//}
//}
//}
