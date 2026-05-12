import { Skeleton } from "@/components/ui/skeleton";

/* ── Summary Header Skeleton ── */
export function SummaryHeaderSkeleton() {
  return (
    <header className="safe-top relative px-5 pb-6 pt-4">
      <div className="mb-5 flex items-center justify-between">
        <div>
          <Skeleton className="mb-1 h-3 w-20" />
          <Skeleton className="h-5 w-28" />
        </div>
        <Skeleton className="h-10 w-10 rounded-full" />
      </div>
      <Skeleton className="h-44 w-full rounded-[2rem]" />
    </header>
  );
}

/* ── Dashboard (subscription list) Skeleton ── */
export function DashboardSkeleton() {
  return (
    <div className="space-y-4 px-5 pt-2">
      {/* Shared Rooms shimmer */}
      <div className="flex gap-3 overflow-hidden">
        <Skeleton className="h-24 w-48 shrink-0 rounded-xl" />
        <Skeleton className="h-24 w-60 shrink-0 rounded-xl" />
      </div>
      {/* Filter bar */}
      <Skeleton className="h-11 w-full rounded-2xl" />
      {/* Cards */}
      {Array.from({ length: 4 }).map((_, i) => (
        <SubscriptionCardSkeleton key={i} />
      ))}
    </div>
  );
}

export function SubscriptionCardSkeleton() {
  return (
    <div className="bg-surface flex items-center gap-3 rounded-2xl p-4">
      <Skeleton className="h-10 w-10 rounded-xl" />
      <div className="flex-1 space-y-2">
        <Skeleton className="h-4 w-28" />
        <Skeleton className="h-3 w-20" />
      </div>
      <Skeleton className="h-5 w-16" />
    </div>
  );
}

/* ── Calendar Skeleton ── */
export function CalendarSkeleton() {
  return (
    <div className="px-5">
      <div className="bg-surface rounded-2xl p-4">
        {/* Month header */}
        <div className="mb-3 flex items-center justify-between">
          <Skeleton className="h-8 w-8 rounded-full" />
          <Skeleton className="h-5 w-32" />
          <Skeleton className="h-8 w-8 rounded-full" />
        </div>
        {/* Weekday headers */}
        <div className="grid grid-cols-7 gap-1 mb-2">
          {Array.from({ length: 7 }).map((_, i) => (
            <Skeleton key={i} className="mx-auto h-3 w-5" />
          ))}
        </div>
        {/* Calendar grid */}
        <div className="grid grid-cols-7 gap-1">
          {Array.from({ length: 35 }).map((_, i) => (
            <Skeleton key={i} className="aspect-square rounded-lg" />
          ))}
        </div>
      </div>
      {/* Upcoming label */}
      <Skeleton className="mt-6 mb-3 h-3 w-24" />
      {/* Cards */}
      {Array.from({ length: 3 }).map((_, i) => (
        <div key={i} className="mb-2">
          <SubscriptionCardSkeleton />
        </div>
      ))}
    </div>
  );
}

/* ── Analytics Skeleton ── */
export function AnalyticsSkeleton() {
  return (
    <div className="space-y-5 px-5">
      {/* Date presets */}
      <div className="flex gap-2">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-7 w-16 shrink-0 rounded-full" />
        ))}
      </div>
      {/* Period label */}
      <Skeleton className="mx-auto h-3 w-20" />
      {/* KPI cards */}
      <div className="grid grid-cols-2 gap-3">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-20 rounded-2xl" />
        ))}
      </div>
      {/* Category section */}
      <Skeleton className="h-48 rounded-2xl" />
      {/* Upcoming */}
      <Skeleton className="h-3 w-24" />
      {Array.from({ length: 3 }).map((_, i) => (
        <SubscriptionCardSkeleton key={i} />
      ))}
    </div>
  );
}

/* ── Settings Skeleton ── */
export function SettingsSkeleton() {
  return (
    <div className="px-5">
      {/* Profile header */}
      <div className="bg-surface mb-5 flex items-center gap-4 rounded-2xl p-5">
        <Skeleton className="h-16 w-16 rounded-full" />
        <div className="flex-1 space-y-2">
          <Skeleton className="h-5 w-32" />
          <Skeleton className="h-3 w-24" />
        </div>
      </div>
      {/* Currency selector */}
      <Skeleton className="mb-5 h-28 rounded-2xl" />
      {/* Settings items */}
      <div className="space-y-2">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-16 rounded-2xl" />
        ))}
      </div>
    </div>
  );
}
