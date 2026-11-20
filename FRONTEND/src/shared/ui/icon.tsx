import type { CSSProperties } from "react";
const paths = {
  grid: "M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z",
  chat: "M21 11.5a8.4 8.4 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.4 8.4 0 0 1-3.8-.9L3 21l1.9-5.7a8.4 8.4 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.4 8.4 0 0 1 3.8-.9h.5a8.5 8.5 0 0 1 8 8z",
  users:
    "M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2 M22 21v-2a4 4 0 0 0-3-3.87 M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8 M16 3.13a4 4 0 0 1 0 7.75",
  ticket:
    "M4 4h16v5a3 3 0 0 0 0 6v5H4v-5a3 3 0 0 0 0-6z M15 4v3 M15 11v2 M15 17v3",
  book: "M12 7v14 M3 3h5a4 4 0 0 1 4 4 4 4 0 0 1 4-4h5v16h-5a4 4 0 0 0-4 2 4 4 0 0 0-4-2H3z",
  bolt: "M13 2 3 14h8l-1 8 11-12h-8z",
  arrow: "M5 12h14 M12 5l7 7-7 7",
  plus: "M12 5v14 M5 12h14",
  search: "M21 21l-4.35-4.35 M19 11a8 8 0 1 1-16 0 8 8 0 0 1 16 0",
  logout: "M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4 M16 17l5-5-5-5 M21 12H9",
  check: "M20 6 9 17l-5-5",
  close: "M18 6 6 18 M6 6l12 12",
  refresh:
    "M20 7v5h-5 M4 17v-5h5 M6 7a7 7 0 0 1 12-1l2 3 M4 15l2 3a7 7 0 0 0 12-1",
  chevron: "M9 5l7 7-7 7",
  edit: "m16 3 5 5-12 12-6 1 1-6z M14 5l5 5",
  send: "m22 2-7 20-4-9-9-4z M22 2 11 13",
  shield: "M12 22s9-4 9-11V4l-9-3-9 3v7c0 7 9 11 9 11 M8 11l3 3 5-5",
  menu: "M3 6h18 M3 12h18 M3 18h18",
  sidebarCollapse: "M3 5h18v14H3z M9 5v14 M16 9l-3 3 3 3",
  sidebarExpand: "M3 5h18v14H3z M9 5v14 M13 9l3 3-3 3",
  globe:
    "M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0 M3 12h18 M12 3a17 17 0 0 1 0 18 17 17 0 0 1 0-18",
  info: "M12 11v6 M12 7h.01 M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0",
  bell: "M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9 M10 21h4",
  download: "M12 3v12 M7 10l5 5 5-5 M4 16v4h16v-4",
  attachment: "m8 13 6-6a2.8 2.8 0 0 1 4 4l-8 8a4.2 4.2 0 0 1-6-6l8-8",
  help: "M9 9a3 3 0 1 1 4 2.8c-1 .4-1 1.2-1 2.2 M12 17h.01 M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0",
  sun: "M12 2v2 M12 20v2 M2 12h2 M20 12h2 M5 5l1.5 1.5 M17.5 17.5 19 19 M5 19l1.5-1.5 M17.5 6.5 19 5 M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0",
} as const;
export type IconName = keyof typeof paths;
export function Icon({
  name,
  size = 20,
  style,
}: {
  name: IconName;
  size?: number;
  style?: CSSProperties;
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      style={style}
    >
      <path d={paths[name]} />
    </svg>
  );
}
