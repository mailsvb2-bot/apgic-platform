import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import {test} from "node:test";
import {fileURLToPath} from "node:url";
import {appendLessonMediaAssignments} from "../app/academy/cloud-media-assignments.ts";

// Lock the confirmed live flow: an author signs into APGIC, OAuth returns
// into this exact draft, the embedded Disk picker browses folders and
// verified imports stay on the existing author-owned API.
test("Academy embedded Yandex Disk workflow preserves canonical OAuth and verified import",()=>{
 const page=readFileSync(fileURLToPath(new URL("../app/academy/page.tsx",import.meta.url)),"utf8");
 const editor=readFileSync(fileURLToPath(new URL("../app/academy/cloud-media-editor.tsx",import.meta.url)),"utf8");
 assert.match(page,/cloud_connected/);
 assert.match(page,/course_id/);
 assert.match(page,/version_id/);
 assert.match(page,/Шаг 1\. Курс/);
 assert.ok(page.indexOf("<CloudMediaEditor")>page.indexOf('<h3>Урок</h3>'));
 assert.match(editor,/\/v1\/academy\/author\/cloud\/yandex\/connect/);
 assert.match(editor,/\/v1\/academy\/author\/cloud\/yandex\/files/);
 assert.match(editor,/cloud_connected/);
 assert.match(editor,/lesson_id=\$\{encodeURIComponent\(lessonId\)\}/);
 assert.match(editor,/query\.get\("lesson_id"\)/);
 // The OAuth effect runs before the library reload effect. The reload must
 // not erase the lesson restored from signed OAuth state.
 assert.ok(page.includes('key={`${course.id}:${course.version_id}`}'));
 assert.doesNotMatch(editor,/setLessonId\("")/);
 assert.match(editor,/browseDisk\("disk:\/"\)/);
 assert.match(editor,/\/v1\/academy\/author\/cloud-media\/import/);
 assert.match(editor,/\/v1\/academy\/author\/courses\/\$\{courseId\}\/versions\/\$\{versionId\}\/media/);
 assert.match(editor,/\/v1\/academy\/courses\/\$\{courseId\}\/versions\/\$\{versionId\}\/media\/assign/);
 assert.match(editor,/videoLessons\.length===0/);
 assert.match(editor,/!lessonId\?<p role="status">/);
});

test("new videos append to selected lesson without changing other lessons or duplicating assignments",()=>{
 const initial=[
  {lesson_id:"a",media_id:"m1",ordinal:1},
  {lesson_id:"b",media_id:"m2",ordinal:1},
 ];
 assert.deepEqual(appendLessonMediaAssignments(initial,"a",["m1","m3","m3","m4"]),[
  ...initial,{lesson_id:"a",media_id:"m3",ordinal:2},{lesson_id:"a",media_id:"m4",ordinal:3}
 ]);
 assert.deepEqual(initial,[{lesson_id:"a",media_id:"m1",ordinal:1},{lesson_id:"b",media_id:"m2",ordinal:1}]);
 assert.deepEqual(appendLessonMediaAssignments(initial,"",["m9"]),initial);
});
