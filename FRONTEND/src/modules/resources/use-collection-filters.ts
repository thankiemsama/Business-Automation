"use client";

import { useMemo, useState } from "react";
import type { ListQuery } from "@/shared/domain/models";
import type { Filters } from "@/shared/api/workspace";

export const emptyFilters: Filters = { priority: "", tags: [], assignee: "" };

export function toggleSelection(ids: string[], id: string) {
  return ids.includes(id) ? ids.filter((item) => item !== id) : [...ids, id];
}

export function useCollectionFilters(limit: number) {
  const [filters, setFilters] = useState(emptyFilters);
  const [page, setPage] = useState(1);
  const [q, setQ] = useState("");
  const [assignment, setAssignment] = useState("");
  const [status, setStatus] = useState("");
  const [checked, setChecked] = useState<string[]>([]);
  const query = useMemo<ListQuery>(
    () => ({
      page,
      limit,
      q,
      status,
      assignment,
      priority: filters.priority,
      tag: filters.tags,
      assignee: filters.assignee,
    }),
    [page, limit, q, status, assignment, filters],
  );
  return {
    filters,
    setFilters,
    page,
    setPage,
    q,
    setQ,
    assignment,
    setAssignment,
    status,
    setStatus,
    checked,
    setChecked,
    query,
  };
}
