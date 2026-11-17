"use client";
import { useEffect, useRef, useState } from "react";
import { errorMessage } from "../api/http-client";

// Callers provide a stable loader; cleanup prevents stale responses after navigation.
export function useQuery<T>(loader: (signal: AbortSignal) => Promise<T>) {
  const [state, setState] = useState<{
    data?: T;
    error?: string;
    loading: boolean;
    loader?: typeof loader;
  }>({ loading: true });
  const [version, setVersion] = useState(0);
  const generation = useRef(0);
  useEffect(() => {
    const controller = new AbortController();
    const current = ++generation.current;
    loader(controller.signal)
      .then((data) => {
        if (!controller.signal.aborted && current === generation.current)
          setState({ data, loading: false, loader });
      })
      .catch((error) => {
        if (!controller.signal.aborted && current === generation.current)
          setState({ error: errorMessage(error), loading: false, loader });
      });
    return () => controller.abort();
  }, [loader, version]);
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
