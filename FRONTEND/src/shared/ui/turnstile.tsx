"use client";
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
} from "react";

// Cloudflare Turnstile proof-of-human widget (CAPTCHA, off by default). It is
// rendered only when NEXT_PUBLIC_NEXA_TURNSTILE_SITE_KEY is set; the backend
// verifies tokens server-side against NEXA_TURNSTILE_SECRET.

declare global {
  interface Window {
    turnstile?: {
      render(element: HTMLElement, options: Record<string, unknown>): string;
      reset(id?: string): void;
      getResponse(id?: string): string;
      remove(id?: string): void;
    };
  }
}

let turnstileReady: Promise<void> | null = null;
function loadTurnstile(): Promise<void> {
  if (turnstileReady) return turnstileReady;
  turnstileReady = new Promise((resolve, reject) => {
    if (window.turnstile) {
      resolve();
      return;
    }
    const script = document.createElement("script");
    script.src =
      "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
    script.async = true;
    script.onload = () => resolve();
    script.onerror = () => {
      turnstileReady = null;
      reject(new Error("turnstile failed to load"));
    };
    document.head.appendChild(script);
  });
  return turnstileReady;
}

export interface TurnstileHandle {
  /** The most recent valid token, empty string until the challenge passes. */
  getToken(): string;
  /** Invalidate the current token so the next submit re-issues one. */
  reset(): void;
}

/**
 * Mounts a Turnstile widget. The holder div sizes itself to the widget; keep
 * it visible on the form so the challenge stays available to the user.
 */
export const Turnstile = forwardRef<TurnstileHandle, { siteKey: string }>(
  function Turnstile({ siteKey }, ref) {
    const holder = useRef<HTMLDivElement>(null);
    const widget = useRef<string>("");

    useEffect(() => {
      let cancelled = false;
      loadTurnstile()
        .then(() => {
          if (cancelled || !holder.current || !window.turnstile) return;
          widget.current = window.turnstile.render(holder.current, {
            sitekey: siteKey,
            theme: "light",
          });
        })
        .catch(() => {
          /* The widget stays empty; the backend only enforces the check when
           * NEXA_TURNSTILE_SECRET is configured, so a failed script load in a
           * dev environment must not block the form. */
        });
      return () => {
        cancelled = true;
      };
    }, [siteKey]);

    useImperativeHandle(
      ref,
      () => ({
        getToken() {
          return window.turnstile
            ? window.turnstile.getResponse(widget.current)
            : "";
        },
        reset() {
          if (window.turnstile) window.turnstile.reset(widget.current);
        },
      }),
      [],
    );

    return <div ref={holder} data-testid="turnstile" />;
  },
);
