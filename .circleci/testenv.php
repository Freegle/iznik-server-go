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
    $rid2 = $r->createUser2Mod($uid, $gid);
    echo "Created User2Mod $rid2\n";
    $cm->create($rid2, $uid, "The plane in Spayne falls mainly on the reign.");
    $rid3 = $r->createUser2Mod($uid, $gid2);
    echo "Created User2Mod $rid3\n";
    $cm->create($rid3, $uid, "The plane in Spayne falls mainly on the reign.");
    list ($rid4, $banned) = $r->createConversation($uid3, $uid);
    echo "Created conversation $rid4\n";
    $rid5 = $r->createUser2Mod($uid2, $gid);
    echo "Created User2Mod $rid5\n";
    $cm->create($rid5, $uid2, "The plane in Spayne falls mainly on the reign.");
    
} else {
    error_log("Chat rooms already exist");
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

# Check for messages - only create if none exist
$existing_messages = $dbhr->preQuery("SELECT COUNT(*) as count FROM messages 
                         INNER JOIN messages_groups ON messages_groups.msgid = messages.id 
                         INNER JOIN `groups` ON messages_groups.groupid = groups.id 
                         WHERE messages.deleted IS NULL AND groups.nameshort = 'FreeglePlayground'");
if ($existing_messages[0]['count'] < 2) {
    error_log("Creating test messages for Go tests (need at least 2)");
    
    # Create first message directly
    $dbhm->preExec("INSERT INTO messages (fromuser, subject, textbody, type, arrival, lat, lng) VALUES (?, 'OFFER: Test Chair (Tuvalu High Street)', 'Test chair available for collection. Good condition.', 'Offer', NOW(), 55.9533, -3.1883)", [$uid]);
    $msg_id1 = $dbhm->lastInsertId();
    if ($msg_id1) {
        # Add to group
        $dbhm->preExec("INSERT INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, 'Approved', NOW())", [$msg_id1, $gid]);
        # Add to spatial index
        $dbhm->preExec("INSERT INTO messages_spatial (msgid, successful, arrival, point) VALUES (?, 1, NOW(), ST_SRID(POINT(-3.1883,55.9533), 3857))", [$msg_id1]);
        if ($pcid) {
            $dbhm->preExec("UPDATE messages SET locationid = ? WHERE id = ?", [$pcid, $msg_id1]);
        }
        error_log("Created message $msg_id1 - Test Chair");
    }
    
    # Create second message directly
    $dbhm->preExec("INSERT INTO messages (fromuser, subject, textbody, type, arrival, lat, lng) VALUES (?, 'OFFER: Test Sofa (Tuvalu High Street)', 'Comfortable sofa, needs new home. Collection only.', 'Offer', NOW(), 55.9533, -3.1883)", [$uid]);
    $msg_id2 = $dbhm->lastInsertId();
    if ($msg_id2) {
        # Add to group
        $dbhm->preExec("INSERT INTO messages_groups (msgid, groupid, collection, arrival) VALUES (?, ?, 'Approved', NOW())", [$msg_id2, $gid]);
        # Add to spatial index
        $dbhm->preExec("INSERT INTO messages_spatial (msgid, successful, arrival, point) VALUES (?, 1, NOW(), ST_SRID(POINT(-3.1883,55.9533), 3857))", [$msg_id2]);
        if ($pcid) {
            $dbhm->preExec("UPDATE messages SET locationid = ? WHERE id = ?", [$pcid, $msg_id2]);
        }
        error_log("Created message $msg_id2 - Test Sofa");
    }
} else {
    error_log("Sufficient messages already exist for Go tests");
}

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
        error_log("Created message $msg_id with spaces in subject");
    }
} else {
    error_log("Message with spaces in subject already exists");
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
(1, 'AtRisk', 'inactive', 'We\'ll stop sending you emails soon...', 'It looks like you've not been active on Freegle for a while. So that we don't clutter your inbox, and to reduce the load on our servers, we'll stop sending you emails soon.\n\nIf you'd still like to get them, then just go to www.ilovefreegle.org and log in to keep your account active.\n\nMaybe you've got something lying around that someone else could use, or perhaps there's something someone else might have?', 249, 14, '5.62', 1),
(4, 'Inactive', 'missing', 'We miss you!', 'We don\'t think you\'ve freegled for a while.  Can we tempt you back?  Just come to https://www.ilovefreegle.org', 4681, 63, '1.35', 1),
(7, 'AtRisk', 'inactive', 'Do you want to keep receiving Freegle mails?', 'It looks like you've not been active on Freegle for a while. So that we don't clutter your inbox, and to reduce the load on our servers, we'll stop sending you emails soon.\r\n\r\nIf you'd still like to get them, then just go to www.ilovefreegle.org and log in to keep your account active.\r\n\r\nMaybe you've got something lying around that someone else could use, or perhaps there's something someone else might have?', 251, 8, '3.19', 1),
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

$existing_jobs = $dbhr->preQuery("SELECT COUNT(*) as count FROM jobs");
if ($existing_jobs[0]['count'] == 0) {
    error_log("Creating test job");
    $dbhm->preExec("INSERT INTO `jobs` (`id`, `location`, `title`, `city`, `state`, `zip`, `country`, `job_type`, `posted_at`, `job_reference`, `company`, `mobile_friendly_apply`, `category`, `html_jobs`, `url`, `body`, `cpc`, `geometry`, `visible`) VALUES
(5, 'Darlaston', 'HGV Technician', 'Darlaston', 'West Midlands', '', 'United Kingdom', 'Full Time', '2021-04-02 00:00:00', '12729_6504119', 'WhatJobs', 'No', 'Automotive/Aerospace', 'No', 'https://click.appcast.io/track/7uzy6k9?cs=jrp&jg=3fp2&bid=kb5ujMfJF7FIbT-u0dJZww==', 'HGV Technician - The Hartshorne Group is one of the leading commercial vehicle distributors for the West Midlands, East Midlands, Derbyshire, Nottinghamshire, Shropshire and Staffordshire. We provide full parts and service facilities for Volvo Truck and Bus as well as new and used sales, plus a diverse range of associated services. We are currently recruiting for a HGV Technician at our Walsall depot. HGV Technician Description: To carry out fault diagnosis, service and repairs to Volvo repair standards. Complete repair order write up, service report sheets and production card information. The successful candidate will have the ability to work under pressure, to actively seek solutions to problems. Good verbal communication skills. Providing excellent customer service is paramount.', '0.1000', ST_GeomFromText('POINT(-2.0355619 52.5733189)', {$dbhr->SRID()}), 1)");
} else {
    error_log("Jobs already exist");
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

# Check for volunteer opportunities
$existing_volunteering = $dbhr->preQuery("SELECT COUNT(*) as count FROM volunteering");
if ($existing_volunteering[0]['count'] == 0) {
    error_log("Creating volunteer opportunity");
    $c = new Volunteering($dbhm, $dbhm);
    $id = $c->create($uid, 'Test vacancy', FALSE, 'Test location', NULL, NULL, NULL, NULL, NULL, NULL);
    $c->setPrivate('pending', 0);
    $start = Utils::ISODate('@' . (time()+6000));
    $end = Utils::ISODate('@' . (time()+6000));
    $c->addDate($start, $end, NULL);
    $c->addGroup($gid);
} else {
    error_log("Volunteer opportunities already exist");
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
