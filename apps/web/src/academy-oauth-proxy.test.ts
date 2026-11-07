import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import {test} from "node:test";
import {fileURLToPath} from "node:url";

// Regression: a server-side fetch with its default redirect mode will follow
// the Yandex 302 and incorrectly render the provider login HTML at apgic.ru.
// The OAuth redirect must reach the browser as a 302 with a Location header.
test("Academy API proxy forwards OAuth redirects without following them", () => {
  const route = fileURLToPath(new URL("../app/v1/[...path]/route.ts", import.meta.url));
  const source = readFileSync(route, "utf8");
  assert.match(source, /await fetch\(target,\s*\{[\s\S]*?redirect:\s*["']manual["']/);
  assert.match(source, /new Response\(response\.body,\s*\{\s*status:\s*response\.status,\s*headers:\s*outgoing\s*\}\)/);
  assert.doesNotMatch(source, /http\.Redirect|oauth\.yandex\.ru\/authorize\?/);
});
