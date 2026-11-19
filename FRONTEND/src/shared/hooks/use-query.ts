"use client";
import { useEffect, useRef, useState } from "react";
import { errorMessage } from "../api/http-client";

// Callers provide a stable loader; cleanup prevents stale responses after navigation.
export function useQuery<T>(
  loader: (signal: AbortSignal) => Promise<T>,
  refreshEvent?: string,
  refreshInterval?: number,
) {
  const [state, setState] = useState<{
    data?: T;
    error?: string;
    loading: boolean;
    loader?: typeof loader;
  }>({ loading: true });
  const [version, setVersion] = useState(0);
  const generation = useRef(0);
  useEffect(() => {
    let controller: AbortController | undefined;
    function fetchData() {
      controller?.abort();
      const active = new AbortController();
      controller = active;
      const current = ++generation.current;
      loader(active.signal)
        .then((data) => {
          if (!active.signal.aborted && current === generation.current)
            setState({ data, loading: false, loader });
        })
        .catch((error) => {
          if (!active.signal.aborted && current === generation.current)
            setState({ error: errorMessage(error), loading: false, loader });
        });
    }
    fetchData();
    if (refreshEvent) window.addEventListener(refreshEvent, fetchData);
    const timer = refreshInterval
      ? window.setInterval(fetchData, refreshInterval)
      : undefined;
    return () => {
      controller?.abort();
      if (refreshEvent) window.removeEventListener(refreshEvent, fetchData);
      window.clearInterval(timer);
    };
  }, [loader, version, refreshEvent, refreshInterval]);
  function reload() {
    setState((previous) => ({ ...previous, error: undefined, loading: true }));
    setVersion((v) => v + 1);
  }
  return {
    ...(state.loader === loader
      ? state
      : { loading: true, data: undefined, error: undefined }),
    reload,
  };
}
