const fs = require("fs");
const p = "D:/GOLANG/data/testrun/harness.mjs";
let s = fs.readFileSync(p, "utf8");

// Restore broken `jsv(.data, c)` / `jsv(.data)` -> `jsv(<recv>.data)` using the
// `const <recv> =` variable on the same line (default to r).
let n = 0;
s = s.replace(/jsv\(\.data(, c)?\)/g, (m) => {
  // m is jsv(.data, c) or jsv(.data)
  return m; // placeholder, replaced below with line-aware pass
});

// Line-aware pass:
const lines = s.split("\n");
for (let i = 0; i < lines.length; i++) {
  if (lines[i].includes("jsv(.data")) {
    const cm = lines[i].match(/const (r\d?|rr|withCode|cc|up) = await api/);
    const recv = cm ? cm[1] : "r";
    lines[i] = lines[i]
      .replace(/jsv\(\.data, c\)/g, `jsv(${recv}.data, 60)`)
      .replace(/jsv\(\.data\)/g, `jsv(${recv}.data, 60)`);
    n++;
  }
}
s = lines.join("\n");
fs.writeFileSync(p, s);
console.log("fixed lines:", n, "| remaining broken jsv(.data):", (s.match(/jsv\(\.data/g) || []).length);
