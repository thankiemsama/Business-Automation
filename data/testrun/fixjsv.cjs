const fs = require("fs");
const p = "D:/GOLANG/data/testrun/harness.mjs";
let s = fs.readFileSync(p, "utf8");

// Fix the one mangled line first.
s = s.replace(
  "jsv(r.data?.governance, .slice(0, 60))",
  "jsv(r.data?.governance, 60)"
);

// Convert remaining fragile `JSON.stringify(rX.data...).slice(0, N)` to jsv(rX.data, N).
// Also handle bare `JSON.stringify(rX.data)` -> jsv(rX.data).
// Match the receiver expression that starts with r, rr, withCode, r2, r4, r5 etc + .data
const re = /JSON\.stringify\((r[0-9]?|rr|withCode)(\.data(\?[.\w]+)?)\)(\.slice\(0, (\d+)\))?/g;
let count = 0;
s = s.replace(re, (m, recv, arg, _chain, slice) => {
  count++;
  return slice ? `jsv(${arg}, ${slice[4]})` : `jsv(${arg})`;
});

fs.writeFileSync(p, s);
console.log("converted", count, "call sites; total jsv now:", (s.match(/jsv\(/g) || []).length);
