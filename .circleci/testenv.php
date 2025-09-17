<?php

namespace Freegle\Iznik;

# Set up gridids for locations already in the locations table; you might do this after importing a bunch of locations
# from a source such as OpenStreetMap (OSM).
require_once dirname(__FILE__) . '/../include/config.php';
require_once(IZNIK_BASE . '/include/db.php');
global $dbhr, $dbhm;

error_log("Setting up Go test environment with defensive checks");

# Check for groups
$g = new Group($dbhr, $dbhm);
$gid = $g->findByShortName('FreeglePlayground');

if (!$gid) {
    # Create FreeglePlayground group
    error_log("Creating FreeglePlayground group");
    $gid = $g->create('FreeglePlayground', Group::GROUP_FREEGLE);
    $g->setPrivate('onhere', 1);
    $g->setPrivate('polyofficial', 'POLYGON((-3.1902622 55.9910847, -3.2472542 55.98263430000001, -3.2863922 55.9761038, -3.3159182 55.9522754, -3.3234712 55.9265089, -3.304932200000001 55.911888, -3.3742832 55.8880206, -3.361237200000001 55.8718436, -3.3282782 55.8729997, -3.2520602 55.8964911, -3.2177282 55.895336, -3.2060552 55.8903307, -3.1538702 55.88648049999999, -3.1305242 55.893411, -3.0989382 55.8972611, -3.0680392 55.9091938, -3.0584262 55.9215076, -3.0982522 55.928048, -3.1037452 55.9418938, -3.1236572 55.9649602, -3.168289199999999 55.9849393, -3.1902622 55.9910847))');
    $g->setPrivate('lat', 55.9533);
    $g->setPrivate('lng', -3.1883);
} else {
    error_log("FreeglePlayground group already exists (id: $gid)");
}

# Check for FreeglePlayground2 group
$gid2 = $g->findByShortName('FreeglePlayground2');
if (!$gid2) {
    error_log("Creating FreeglePlayground2 group");
    $gid2 = $g->create('FreeglePlayground2', Group::GROUP_FREEGLE);
    $g->setPrivate('onhere', 1);
    $g->setPrivate('contactmail', 'contact@test.com');
    $g->setPrivate('namefull', 'Freegle Playground2');
} else {
    error_log("FreeglePlayground2 group already exists (id: $gid2)");
}

# Check for locations
$l = new Location($dbhr, $dbhm);
$existing_location = $dbhr->preQuery("SELECT id FROM locations WHERE name = ? LIMIT 1", ['Central']);
if (!$existing_location) {
    error_log("Creating locations");
    $l->copyLocationsToPostgresql();
    $areaid = $l->create(NULL, 'Central', 'Polygon', 'POLYGON((-3.217620849609375 55.9565040997114,-3.151702880859375 55.9565040997114,-3.151702880859375 55.93304863776238,-3.217620849609375 55.93304863776238,-3.217620849609375 55.9565040997114))');
    $pcid = $l->create(NULL, 'EH3 6SS', 'Postcode', 'POINT(-3.205333 55.957571)');
    $l->copyLocationsToPostgresql(FALSE);
} else {
    error_log("Locations already exist");
    # Get existing postcode ID
    $existing_pc = $dbhr->preQuery("SELECT id FROM locations WHERE name = ? AND type = ? LIMIT 1", ['EH3 6SS', 'Postcode']);
    $pcid = $existing_pc ? $existing_pc[0]['id'] : NULL;
}

# Check for test users
$u = new User($dbhr, $dbhm);
$existing_users = $dbhr->preQuery("SELECT id FROM users WHERE fullname = ? AND deleted IS NULL", ['Test User']);

if (!$existing_users) {
    error_log("Creating Test User");
    $uid = $u->create('Test', 'User', 'Test User');
    $u->addEmail('test@test.com');
    $ouremail = $u->inventEmail();
    $u->addEmail($ouremail, 0, FALSE);
    $u->addLogin(User::LOGIN_NATIVE, NULL, 'freegle');
    $u->addMembership($gid);
    $u->setMembershipAtt($gid, 'ourPostingStatus', Group::POSTING_DEFAULT);
} else {
    error_log("Test User already exists");
    $uid = $existing_users[0]['id'];
    # Ensure user is in the group
    $existing_membership = $dbhr->preQuery("SELECT id FROM memberships WHERE userid = ? AND groupid = ?", [$uid, $gid]);
    if (!$existing_membership) {
        $u = new User($dbhr, $dbhm, $uid);
        $u->addMembership($gid);
        $u->setMembershipAtt($gid, 'ourPostingStatus', Group::POSTING_DEFAULT);
    }
}

# Check for isochrone
if ($pcid) {
    $existing_isochrone = $dbhr->preQuery("SELECT id FROM isochrones_users WHERE userid = ? LIMIT 1", [$uid]);
    if (!$existing_isochrone) {
        error_log("Creating isochrone for user $uid");
        $i = new Isochrone($dbhr, $dbhm);
        $id = $i->create($uid, Isochrone::WALK, Isochrone::MAX_TIME, NULL, $pcid);
    } else {
        error_log("Isochrone already exists for user $uid");
    }
}

# Check for moderator user
$existing_mod = $dbhr->preQuery("SELECT id FROM users WHERE deleted IS NULL AND id IN (SELECT userid FROM memberships WHERE role = ?)", [User::ROLE_MODERATOR]);
if (!$existing_mod) {
    error_log("Creating moderator user");
    $uid2 = $u->create('Test', 'User', NULL);
    $u->addEmail('testmod@test.com');
    $u->addLogin(User::LOGIN_NATIVE, NULL, 'freegle');
    $u->addMembership($gid, User::ROLE_MODERATOR);
    $u->setMembershipAtt($gid, 'ourPostingStatus', Group::POSTING_DEFAULT);
} else {
    error_log("Moderator user already exists");
    $uid2 = $existing_mod[0]['id'];
}

# Check for additional test user
$existing_users3 = $dbhr->preQuery("SELECT COUNT(*) as count FROM users WHERE deleted IS NULL");
if ($existing_users3[0]['count'] < 3) {
    error_log("Creating additional test user");
    $uid3 = $u->create('Test', 'User', NULL);
} else {
    # Get the third user
    $users = $dbhr->preQuery("SELECT id FROM users WHERE deleted IS NULL ORDER BY id LIMIT 3");
    $uid3 = count($users) >= 3 ? $users[2]['id'] : $uid;
}

# Check for deleted user
$existing_deleted = $dbhr->preQuery("SELECT id FROM users WHERE deleted IS NOT NULL LIMIT 1");
if (!$existing_deleted) {
    error_log("Creating deleted user");
    $uid4 = $u->create('Test', 'User', NULL);
    $u->setPrivate('deleted', '2024-01-01');
} else {
    error_log("Deleted user already exists");
    $uid4 = $existing_deleted[0]['id'];
}

# Check for Support user
$existing_support = $dbhr->preQuery("SELECT id FROM users WHERE systemrole = ? AND deleted IS NULL LIMIT 1", [User::SYSTEMROLE_SUPPORT]);
if (!$existing_support) {
    error_log("Creating Support user");
    $uid5 = $u->create('Support', 'User', NULL);
    $u->addEmail('testsupport@test.com');
    $u->addLogin(User::LOGIN_NATIVE, NULL, 'freegle');
    $u->addMembership($gid, User::ROLE_MODERATOR);
    $u->setPrivate('systemrole', User::SYSTEMROLE_SUPPORT);
} else {
    error_log("Support user already exists");
    $uid5 = $existing_support[0]['id'];
}

# Check for chat rooms - this is complex so let's just check if any exist
$existing_chats = $dbhr->preQuery("SELECT COUNT(*) as count FROM chat_rooms");
if ($existing_chats[0]['count'] == 0) {
    error_log("Creating chat rooms");
    $r = new ChatRoom($dbhr, $dbhm);
    list ($rid, $banned) = $r->createConversation($uid, $uid2);
    echo "Created conversation $rid\n";
    $cm = new ChatMessage($dbhr, $dbhm);
    $cm->create($rid, $uid, "The plane in Spayne falls mainly on the reign.");
    # Update latestmessage timestamp for user2user chat
    $dbhm->preExec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", [$rid]);

    $rid2 = $r->createUser2Mod($uid, $gid);
    echo "Created User2Mod $rid2\n";
    $cm->create($rid2, $uid, "The plane in Spayne falls mainly on the reign.");
    # Update latestmessage timestamp for user2mod chat
    $dbhm->preExec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", [$rid2]);

    $rid3 = $r->createUser2Mod($uid, $gid2);
    echo "Created User2Mod $rid3\n";
    $cm->create($rid3, $uid, "The plane in Spayne falls mainly on the reign.");
    # Update latestmessage timestamp
    $dbhm->preExec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", [$rid3]);

    list ($rid4, $banned) = $r->createConversation($uid3, $uid);
    echo "Created conversation $rid4\n";
    # Update latestmessage timestamp
    $dbhm->preExec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", [$rid4]);

    $rid5 = $r->createUser2Mod($uid2, $gid);
    echo "Created User2Mod $rid5\n";
    $cm->create($rid5, $uid2, "The plane in Spayne falls mainly on the reign.");
    # Update latestmessage timestamp
    $dbhm->preExec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", [$rid5]);
    
} else {
    error_log("Chat rooms already exist");
    # Update existing chat rooms to have recent timestamps for Go tests
    $dbhm->preExec("UPDATE chat_rooms SET latestmessage = NOW() WHERE latestmessage IS NULL OR latestmessage < DATE_SUB(NOW(), INTERVAL 30 DAY)");
    error_log("Updated existing chat room timestamps");

    # Ensure the Test User has the required chat relationships for Go tests
    # Check if Test User has User2User chat as user1
    $test_user_u2u = $dbhr->preQuery("SELECT id FROM chat_rooms WHERE user1 = ? AND chattype = 'User2User' LIMIT 1", [$uid]);
    if (!$test_user_u2u) {
        error_log("Creating User2User chat for Test User $uid");
        # Create a conversation where Test User is user1
        list ($rid_test, $banned) = $r->createConversation($uid, $uid2);
        $cm = new ChatMessage($dbhr, $dbhm);
        $cm->create($rid_test, $uid, "Test User chat message for Go tests");
        $dbhm->preExec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", [$rid_test]);
        error_log("Created User2User chat $rid_test for Test User");
    }

    # Check if Test User has User2Mod chat as user1
    $test_user_u2m = $dbhr->preQuery("SELECT id FROM chat_rooms WHERE user1 = ? AND chattype = 'User2Mod' LIMIT 1", [$uid]);
    if (!$test_user_u2m) {
        error_log("Creating User2Mod chat for Test User $uid");
        $rid_test_mod = $r->createUser2Mod($uid, $gid);
        $cm = new ChatMessage($dbhr, $dbhm);
        $cm->create($rid_test_mod, $uid, "Test User mod chat message for Go tests");
        $dbhm->preExec("UPDATE chat_rooms SET latestmessage = NOW() WHERE id = ?", [$rid_test_mod]);
        error_log("Created User2Mod chat $rid_test_mod for Test User");
    }
}

# Check for moderator-initiated User2Mod chat (needed for GetChatFromModToGroup test)  
$existing_mod_chat = $dbhr->preQuery("SELECT cr.id FROM chat_rooms cr INNER JOIN users u ON u.id = cr.user1 WHERE cr.chattype = 'User2Mod' AND u.systemrole != 'User' LIMIT 1");
if (!$existing_mod_chat) {
    error_log("Creating moderator-initiated User2Mod chat");
    # Create additional User2Mod chat where moderator is user1 (needed for GetChatFromModToGroup test)
    # We need to manually create this because createUser2Mod puts the user as user1, not the mod
    $dbhm->preExec("INSERT INTO chat_rooms (user1, groupid, chattype, latestmessage) VALUES (?, ?, 'User2Mod', NOW())", [$uid2, $gid]);
    $mod_chat_id = $dbhm->lastInsertId();
    if ($mod_chat_id) {
        $cm = new ChatMessage($dbhr, $dbhm);
        $cm->create($mod_chat_id, $uid2, "Moderator initiated chat");
        error_log("Created User2Mod chat $mod_chat_id with moderator as user1");
    }
} else {
    error_log("Moderator-initiated User2Mod chat already exists");
}

# Check for newsfeed items
$existing_newsfeed = $dbhr->preQuery("SELECT COUNT(*) as count FROM newsfeed");
if ($existing_newsfeed[0]['count'] == 0) {
    error_log("Creating newsfeed items");
    $n = new Newsfeed($dbhr, $dbhm);
    $nid = $n->create(Newsfeed::TYPE_MESSAGE, $uid, "This is a test post mentioning https://www.ilovefreegle.org");
    $nid2 = $n->create(Newsfeed::TYPE_MESSAGE, $uid, "This is a test reply mentioning https://www.ilovefreegle.org", NULL, NULL, $nid);
} else {
    error_log("Newsfeed items already exist");
}

# Ensure messages meet Go API requirements: recent arrival (within 31 days), collection='Approved',
# messages_groups.deleted=0, users.deleted IS NULL, no outcomes (or Taken/Received)
$api_messages = $dbhr->preQuery("SELECT COUNT(*) as count FROM messages_groups mg
    INNER JOIN messages m ON m.id = mg.msgid
    INNER JOIN users u ON u.id = m.fromuser
    INNER JOIN `groups` g ON g.id = mg.groupid
    LEFT JOIN messages_outcomes mo ON mo.msgid = mg.msgid
    WHERE g.nameshort = 'FreeglePlayground'
    AND mg.collection = 'Approved'
    AND mg.deleted = 0
    AND u.deleted IS NULL
    AND mg.arrival >= DATE_SUB(NOW(), INTERVAL 31 DAY)
    AND (mo.id IS NULL OR mo.outcome IN ('Taken', 'Received'))");

$api_message_count = $api_messages[0]['count'];
error_log("Found $api_message_count messages meeting Go API requirements (recent, approved, not deleted, valid outcomes)");

if (true) { # Force message creation for testing
    error_log("Creating messages that meet Go API requirements (need at least 2, have $api_message_count)");

    # Create sufficient messages to reach at least 2
    $messages_to_create = max(2, 2 - $api_message_count); # Always create at least 2

    for ($i = 1; $i <= $messages_to_create; $i++) {
        $subject = "OFFER: Test Item $i (Go Test Location)";
        $body = "Test item $i available for collection. Good condition. Created for Go tests.";

        error_log("Creating message $i: $subject");
        $dbhm->preExec("INSERT INTO messages (fromuser, subject, textbody, type, arrival, lat, lng) VALUES (?, ?, ?, 'Offer', NOW(), 55.9533, -3.1883)", [$uid, $subject, $body]);
        $msg_id = $dbhm->lastInsertId();

        if ($msg_id) {
            # Add to group with Approved status and recent arrival (within last 31 days)
            $dbhm->preExec("INSERT INTO messages_groups (msgid, groupid, collection, arrival, deleted) VALUES (?, ?, 'Approved', NOW(), 0)", [$msg_id, $gid]);
            # Add to spatial index
            $dbhm->preExec("INSERT INTO messages_spatial (msgid, successful, arrival, point) VALUES (?, 1, NOW(), ST_SRID(POINT(-3.1883,55.9533), 3857))", [$msg_id]);
            if ($pcid) {
                $dbhm->preExec("UPDATE messages SET locationid = ? WHERE id = ?", [$pcid, $msg_id]);
            }

            # Index the message for search
            $m = new Message($dbhr, $dbhm, $msg_id);
            $m->index();
            error_log("Created and indexed message $msg_id - $subject");
        }
    }
} else {
    error_log("Sufficient messages exist that meet Go API requirements ($api_message_count >= 2)");

    # Ensure all FreeglePlayground messages meet Go API requirements
    $all_playground_messages = $dbhr->preQuery("SELECT mg.msgid FROM messages_groups mg
                                               INNER JOIN `groups` g ON mg.groupid = g.id
                                               WHERE g.nameshort = 'FreeglePlayground'");
    foreach ($all_playground_messages as $msg) {
        # Update to meet all Go API requirements
        $dbhm->preExec("UPDATE messages_groups SET collection = 'Approved', arrival = NOW(), deleted = 0 WHERE msgid = ?", [$msg['msgid']]);
        $dbhm->preExec("UPDATE messages SET fromuser = ? WHERE id = ?", [$uid, $msg['msgid']]);
        # Ensure no problematic outcomes exist (delete any non-Taken/Received outcomes)
        $dbhm->preExec("DELETE FROM messages_outcomes WHERE msgid = ? AND outcome NOT IN ('Taken', 'Received')", [$msg['msgid']]);
        error_log("Updated message " . $msg['msgid'] . " to meet Go API requirements");
    }

    # Ensure existing messages are properly indexed
    $unindexed = $dbhr->preQuery("SELECT m.id FROM messages m
                                  INNER JOIN messages_groups mg ON mg.msgid = m.id
                                  INNER JOIN `groups` g ON mg.groupid = g.id
                                  LEFT JOIN messages_index mi ON mi.msgid = m.id
                                  WHERE g.nameshort = 'FreeglePlayground' AND mg.collection = 'Approved'
                                  AND m.deleted IS NULL AND mi.msgid IS NULL");
    foreach ($unindexed as $msg) {
        $m = new Message($dbhr, $dbhm, $msg['id']);
        $m->index();
        error_log("Indexed existing message " . $msg['id']);
    }
}

# Verify final message count using same query as Go API
$final_api_messages = $dbhr->preQuery("SELECT COUNT(*) as count FROM messages_groups mg
    INNER JOIN messages m ON m.id = mg.msgid
    INNER JOIN users u ON u.id = m.fromuser
    INNER JOIN `groups` g ON g.id = mg.groupid
    LEFT JOIN messages_outcomes mo ON mo.msgid = mg.msgid
    WHERE g.nameshort = 'FreeglePlayground'
    AND mg.collection = 'Approved'
    AND mg.deleted = 0
    AND u.deleted IS NULL
    AND mg.arrival >= DATE_SUB(NOW(), INTERVAL 31 DAY)
    AND (mo.id IS NULL OR mo.outcome IN ('Taken', 'Received'))");
error_log("Final Go API compliant message count: " . $final_api_messages[0]['count']);

# Check for messages with spaces in subject (needed for GetMessage/LoveJunk test)
$existing_spaced_message = $dbhr->preQuery("SELECT messages.id FROM messages INNER JOIN messages_groups ON messages_groups.msgid = messages.id INNER JOIN messages_spatial ON messages_spatial.msgid = messages.id WHERE subject LIKE '% %' LIMIT 1");
if (!$existing_spaced_message) {
    error_log("Creating message with spaces in subject for LoveJunk test");
    # Create a simple message with spaces in subject
    $dbhm->preExec("INSERT INTO messages (fromuser, subject, textbody, type, arrival, lat, lng) VALUES (?, 'OFFER: Test Sofa', 'Test message body with spaces in subject', 'Offer', NOW(), 55.9533, -3.1883)", [$uid]);
    $msg_id = $dbhm->lastInsertId();
    if ($msg_id && $pcid) {
        # Add to group.
        $dbhm->preExec("INSERT INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, 'Approved', NOW())", [$msg_id, $gid]);

        # Add to spatial index
        $dbhm->preExec("INSERT INTO messages_spatial (msgid, successful, arrival, point) VALUES (?, 1, NOW(), ST_SRID(POINT(-3.1883,55.9533), 3857));", [$msg_id]);
        
        # Index the message for search
        $m_spaced = new Message($dbhr, $dbhm, $msg_id);
        $m_spaced->index();
        error_log("Created message $msg_id with spaces in subject");
    }
} else {
    error_log("Message with spaces in subject already exists");
    # Index the existing spaced message if not already indexed
    $spaced_msg_id = $existing_spaced_message[0]['id'];
    $spaced_indexed = $dbhr->preQuery("SELECT msgid FROM messages_index WHERE msgid = ? LIMIT 1", [$spaced_msg_id]);
    if (!$spaced_indexed) {
        $m_existing = new Message($dbhr, $dbhm, $spaced_msg_id);
        $m_existing->index();
        error_log("Indexed existing spaced message $spaced_msg_id");
    }
}

# Update spatial index if needed
$m = new Message($dbhr, $dbhm);
$m->updateSpatialIndex();

# Check for items
$existing_items = $dbhr->preQuery("SELECT COUNT(*) as count FROM items WHERE name = ?", ['chair']);
if ($existing_items[0]['count'] == 0) {
    error_log("Creating test items");
    $i = new Item($dbhr, $dbhm);
    $i->create('chair');
} else {
    error_log("Test items already exist");
}

# Insert various test data if not exists
$dbhm->preExec("INSERT ignore INTO `spam_keywords` (`id`, `word`, `exclude`, `action`, `type`) VALUES (8, 'viagra', NULL, 'Spam', 'Literal'), (76, 'weight loss', NULL, 'Spam', 'Literal'), (77, 'spamspamspam', NULL, 'Review', 'Literal');");
$dbhm->preExec('REPLACE INTO `spam_keywords` (`id`, `word`, `exclude`, `action`, `type`) VALUES (272, \'(?<!\\\\bwater\\\\W)\\\\bbutt\\\\b(?!\\\\s+rd)\', NULL, \'Review\', \'Regex\');');
$dbhm->preExec("INSERT IGNORE INTO `locations` (`id`, `osm_id`, `name`, `type`, `osm_place`, `geometry`, `ourgeometry`, `gridid`, `postcodeid`, `areaid`, `canon`, `popularity`, `osm_amenity`, `osm_shop`, `maxdimension`, `lat`, `lng`, `timestamp`) VALUES
  (1687412, '189543628', 'SA65 9ET', 'Postcode', 0, ST_GeomFromText('POINT(-4.939858 52.006292)', {$dbhr->SRID()}), NULL, NULL, NULL, NULL, 'sa659et', 0, 0, 0, '0.002916', '52.006292', '-4.939858', '2016-08-23 06:01:25');");
$dbhm->preExec("INSERT IGNORE INTO `paf_addresses` (`id`, `postcodeid`, `udprn`) VALUES   (102367696, 1687412, 50464672);");
$dbhm->preExec("INSERT IGNORE INTO weights (name, simplename, weight, source) VALUES ('2 seater sofa', 'sofa', 37, 'FRN 2009');");
$dbhm->preExec("INSERT IGNORE INTO spam_countries (country) VALUES ('Cameroon');");
$dbhm->preExec("INSERT IGNORE INTO spam_whitelist_links (domain, count) VALUES ('users.ilovefreegle.org', 3);");
$dbhm->preExec("INSERT IGNORE INTO spam_whitelist_links (domain, count) VALUES ('freegle.in', 3);");
$dbhm->preExec("INSERT IGNORE INTO towns (name, lat, lng, position) VALUES ('Edinburgh', 55.9500,-3.2000, ST_GeomFromText('POINT (-3.2000 55.9500)', {$dbhr->SRID()}));");

# Insert other test data with IGNORE
$existing_engage = $dbhr->preQuery("SELECT COUNT(*) as count FROM engage_mails");
if ($existing_engage[0]['count'] == 0) {
    $dbhm->preExec("INSERT INTO `engage_mails` (`id`, `engagement`, `template`, `subject`, `text`, `shown`, `action`, `rate`, `suggest`) VALUES
(1, 'AtRisk', 'inactive', 'We\'ll stop sending you emails soon...', 'It looks like you\'ve not been active on Freegle for a while. So that we don\'t clutter your inbox, and to reduce the load on our servers, we\'ll stop sending you emails soon.\n\nIf you\'d still like to get them, then just go to www.ilovefreegle.org and log in to keep your account active.\n\nMaybe you\'ve got something lying around that someone else could use, or perhaps there\'s something someone else might have?', 249, 14, '5.62', 1),
(4, 'Inactive', 'missing', 'We miss you!', 'We don\'t think you\'ve freegled for a while.  Can we tempt you back?  Just come to https://www.ilovefreegle.org', 4681, 63, '1.35', 1),
(7, 'AtRisk', 'inactive', 'Do you want to keep receiving Freegle mails?', 'It looks like you\'ve not been active on Freegle for a while. So that we don\'t clutter your inbox, and to reduce the load on our servers, we\'ll stop sending you emails soon.\r\n\r\nIf you\'d still like to get them, then just go to www.ilovefreegle.org and log in to keep your account active.\r\n\r\nMaybe you\'ve got something lying around that someone else could use, or perhaps there\'s something someone else might have?', 251, 8, '3.19', 1),
(10, 'Inactive', 'missing', 'Time for a declutter?', 'We don\'t think you\'ve freegled for a while.  Can we tempt you back?  Just come to https://www.ilovefreegle.org', 1257, 8, '0.64', 1),
(13, 'Inactive', 'missing', 'Anything Freegle can help you get?', 'We don\'t think you\'ve freegled for a while.  Can we tempt you back?  Just come to https://www.ilovefreegle.org', 1366, 5, '0.37', 1);");
}

$existing_search = $dbhr->preQuery("SELECT COUNT(*) as count FROM search_terms");
if ($existing_search[0]['count'] == 0) {
    $dbhm->preExec("INSERT INTO `search_terms` (`id`, `term`, `count`) VALUES
(3, '', 92),
(6, '_term', 1),
(9, '-', 1),
(12, '- offer: blue badge road atlas', 1),
(15, '-- end of posted message. the following text has been added by group moderators ', 2),
(18, '-wanted', 2),
(21, ',', 6),
(24, ', ,garden tools', 1),
(27, ', dinning table ,curtains', 1),
(30, ',:', 1),
(33, ',curtains', 1),
(36, ',guitar', 1),
(39, ',ixer', 1),
(42, ':', 1),
(45, ': offered: luxury xmas jigsaw (cv2)', 2),
(48, '?', 1),
(51, '?rollater', 2),
(54, '?rolletar', 1),
(57, '.', 16),
(60, '. beds', 1);");
}

# Ensure jobs meet Go API requirements: cpc >= 0.10, visible = 1, proper category
# Go test queries at lat=52.5833189&lng=-2.0455619
$test_job_api_requirements = $dbhr->preQuery("SELECT COUNT(*) as count FROM jobs WHERE
    cpc >= 0.10 AND visible = 1 AND category IS NOT NULL
    AND ST_Distance(geometry, ST_SRID(POINT(-2.0455619, 52.5833189), {$dbhr->SRID()})) < 50000");

if (true) { # Force job creation for testing
    error_log("Creating jobs that meet Go API requirements (cpc >= 0.10, visible = 1, category not null)");

    # Create job at exact test coordinates with all required fields for Go API
    $dbhm->preExec("INSERT IGNORE INTO `jobs` (`location`, `title`, `city`, `state`, `zip`, `country`, `job_type`, `posted_at`, `job_reference`, `company`, `mobile_friendly_apply`, `category`, `html_jobs`, `url`, `body`, `cpc`, `geometry`, `visible`) VALUES
('Test Location GoAPI', 'Go API Test Job', 'Test City', 'Test State', '', 'United Kingdom', 'Full Time', NOW(), 'GOAPI_001', 'TestCompany', 'No', 'Technology', 'No', 'https://example.com/goapi-job', 'Test job for Go API requirements. CPC >= 0.10, visible = 1, has category.', '0.15', ST_GeomFromText('POINT(-2.0455619 52.5833189)', {$dbhr->SRID()}), 1)");

    # Create a second job slightly offset for variety
    $dbhm->preExec("INSERT IGNORE INTO `jobs` (`location`, `title`, `city`, `state`, `zip`, `country`, `job_type`, `posted_at`, `job_reference`, `company`, `mobile_friendly_apply`, `category`, `html_jobs`, `url`, `body`, `cpc`, `geometry`, `visible`) VALUES
('Nearby Location', 'Nearby Test Job', 'Test City', 'Test State', '', 'United Kingdom', 'Part Time', NOW(), 'GOAPI_002', 'TestCompany2', 'No', 'Engineering', 'No', 'https://example.com/goapi-job2', 'Second test job near the test coordinates.', '0.12', ST_GeomFromText('POINT(-2.0465619 52.5843189)', {$dbhr->SRID()}), 1)");

    error_log("Created jobs meeting Go API requirements");
} else {
    error_log("Jobs already exist that meet Go API requirements (count: " . $test_job_api_requirements[0]['count'] . ")");

    # Ensure existing jobs meet the requirements
    $dbhm->preExec("UPDATE jobs SET visible = 1, cpc = GREATEST(cpc, 0.10) WHERE cpc < 0.10 OR visible != 1");
    $dbhm->preExec("UPDATE jobs SET category = 'General' WHERE category IS NULL OR category = ''");
    error_log("Updated existing jobs to meet API requirements");
}

# This is the critical part - create address for the user
$existing_address = $dbhr->preQuery("SELECT id FROM users_addresses WHERE userid = ? LIMIT 1", [$uid]);
if (!$existing_address) {
    error_log("Creating address for user $uid");
    $a = new Address($dbhr, $dbhm);
    $pafs = $dbhr->preQuery("SELECT * FROM paf_addresses LIMIT 1;");
    foreach ($pafs as $paf) {
        $aid = $a->create($uid, $paf['id'], "Test desc");
        error_log("Created address $aid for user $uid");
    }
} else {
    error_log("Address already exists for user $uid");
}

# Check for sessions
$existing_sessions = $dbhr->preQuery("SELECT COUNT(*) as count FROM sessions WHERE userid IN (?, ?, ?)", [$uid, $uid2, $uid5]);
if ($existing_sessions[0]['count'] < 3) {
    error_log("Creating user sessions");
    $s = new Session($dbhr, $dbhm);
    
    # Only create sessions if they don't exist
    $existing_session_uid = $dbhr->preQuery("SELECT id FROM sessions WHERE userid = ? LIMIT 1", [$uid]);
    if (!$existing_session_uid) {
        $s->create($uid);
    }
    
    $existing_session_uid2 = $dbhr->preQuery("SELECT id FROM sessions WHERE userid = ? LIMIT 1", [$uid2]);
    if (!$existing_session_uid2) {
        $s->create($uid2);
    }
    
    $existing_session_uid5 = $dbhr->preQuery("SELECT id FROM sessions WHERE userid = ? LIMIT 1", [$uid5]);
    if (!$existing_session_uid5) {
        $s->create($uid5);
    }
} else {
    error_log("User sessions already exist");
}

# Ensure volunteer opportunities meet Go test requirements: pending=0, deleted=0, heldby IS NULL, with dates
$go_api_volunteering = $dbhr->preQuery("SELECT COUNT(*) as count FROM volunteering v
    INNER JOIN volunteering_dates vd ON vd.volunteeringid = v.id
    WHERE v.pending = 0 AND v.deleted = 0 AND v.heldby IS NULL");

if ($go_api_volunteering[0]['count'] == 0) {
    error_log("Creating volunteer opportunity that meets Go API requirements");
    $c = new Volunteering($dbhm, $dbhm);
    $id = $c->create($uid, 'Go Test Volunteer Opportunity', FALSE, 'Test location for Go tests', NULL, NULL, NULL, NULL, NULL, NULL);

    # Ensure it meets Go test criteria
    $c->setPrivate('pending', 0);    # Not pending
    $c->setPrivate('deleted', 0);    # Not deleted
    $c->setPrivate('heldby', NULL);  # Not held by anyone

    # Add dates (required by Go test query)
    $start = Utils::ISODate('@' . (time()+6000));
    $end = Utils::ISODate('@' . (time()+12000));
    $c->addDate($start, $end, NULL);

    # Link to group
    $c->addGroup($gid);
    error_log("Created volunteer opportunity $id meeting Go API requirements");
} else {
    error_log("Volunteer opportunities already exist that meet Go API requirements (count: " . $go_api_volunteering[0]['count'] . ")");

    # Ensure existing volunteer opportunities meet the criteria
    $dbhm->preExec("UPDATE volunteering SET pending = 0, deleted = 0, heldby = NULL WHERE pending != 0 OR deleted != 0 OR heldby IS NOT NULL");
    error_log("Updated existing volunteer opportunities to meet Go API requirements");
}

# Check for community events
$existing_events = $dbhr->preQuery("SELECT COUNT(*) as count FROM communityevents");
if ($existing_events[0]['count'] == 0) {
    error_log("Creating community event");
    $c = new CommunityEvent($dbhm, $dbhm);
    $id = $c->create($uid, 'Test event', 'Test location', NULL, NULL, NULL, NULL, NULL);
    $c->setPrivate('pending', 0);
    $start = Utils::ISODate('@' . (time()+6000));
    $end = Utils::ISODate('@' . (time()+6000));
    $c->addDate($start, $end, NULL);
    $c->addGroup($gid);
} else {
    error_log("Community events already exist");
}

# Insert remaining test data
$dbhm->preExec("INSERT IGNORE INTO partners_keys (`partner`, `key`, `domain`) VALUES ('lovejunk', 'testkey123', 'localhost');");
$dbhm->preExec("INSERT IGNORE INTO link_previews (`url`, `title`, `description`) VALUES ('https://www.ilovefreegle.org', 'Freegle', 'Freegle is a UK-wide umbrella organisation for local free reuse groups. We help groups to get started, provide support and advice, and help promote free reuse to the public.');");

error_log("Go test environment setup complete");

# Skip swagger generation when running in apiv1 container during CircleCI tests
# The swagger generation is handled by the Go tests themselves in the apiv2 container
error_log("Skipping swagger generation in PHP test environment setup (handled by Go tests)");
