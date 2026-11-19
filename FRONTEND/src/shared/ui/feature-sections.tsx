"use client";

import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { Suspense, type ReactNode } from "react";
import { Loading } from "./primitives";

interface FeatureSection {
  id: string;
  label: string;
  content: ReactNode;
}

export function FeatureSections({
  label,
  sections,
}: {
  label: string;
  sections: readonly [FeatureSection, ...FeatureSection[]];
}) {
  return (
    <Suspense fallback={<Loading />}>
      <SectionNavigation label={label} sections={sections} />
    </Suspense>
  );
}

function SectionNavigation({
  label,
  sections,
}: {
  label: string;
  sections: readonly [FeatureSection, ...FeatureSection[]];
}) {
  const path = usePathname();
  const search = useSearchParams();
  const active =
    sections.find((section) => section.id === search.get("section")) ??
    sections[0];

  return (
    <div className="feature-layout">
      <nav className="feature-navigation" aria-label={label}>
        {sections.map((section) => {
          const params = new URLSearchParams(search.toString());
          params.set("section", section.id);
          return (
            <Link
              key={section.id}
              href={path + "?" + params.toString()}
              scroll={false}
              aria-current={active.id === section.id ? "page" : undefined}
            >
              {section.label}
            </Link>
          );
        })}
      </nav>
      <div className="feature-content" key={active.id}>
        {active.content}
      </div>
    </div>
  );
}
