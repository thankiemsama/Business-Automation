"use client";
import { NextIntlClientProvider } from "next-intl";
import type { ReactNode } from "react";
import type { messages } from "./messages";

export function LocaleProvider({
  locale,
  messages: dictionary,
  children,
}: {
  locale: "vi" | "en";
  messages: (typeof messages)["vi" | "en"];
  children: ReactNode;
}) {
  return (
    <NextIntlClientProvider
      locale={locale}
      messages={dictionary}
      timeZone="Asia/Ho_Chi_Minh"
    >
      {children}
    </NextIntlClientProvider>
  );
}
