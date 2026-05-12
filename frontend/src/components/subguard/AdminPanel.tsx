import { useEffect, useRef, useState } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { BarChart3 } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { toast } from "sonner";
import {
  ChevronLeft,
  Copy,
  Edit3,
  Heart,
  Link2,
  Plus,
  Send,
  Trash2,
  TrendingUp,
  Users,
  Layers,
  Wallet,
  ListChecks,
  Activity,
  Star,
} from "lucide-react";
import {
  AreaChart,
  Area,
  BarChart,
  Bar,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
  CartesianGrid,
  Cell,
} from "recharts";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { SERVICE_CATEGORIES } from "@/lib/mockData";

import {
  adminCatalog,
  adminGlobalSettings,
  adminKpis,
  liveMetrics,
  userGrowth7d,
  popularServices,
  funnelSteps,
  deepLinkStats,
  type AdminCatalogService,
} from "@/lib/mockAdminData";

const fmtNum = (n: number) =>
  new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 }).format(n);
const fmtUsd = (n: number) =>
  new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 0,
  }).format(n);

function KpiCard({
  Icon,
  label,
  value,
  delta,
}: {
  Icon: typeof Users;
  label: string;
  value: string;
  delta?: string;
}) {
  return (
    <Card className="border-white/10 bg-gradient-to-br from-surface to-surface-elevated/40 p-4">
      <div className="flex items-center justify-between">
        <div className="bg-surface-elevated flex h-9 w-9 items-center justify-center rounded-xl">
          <Icon className="h-4 w-4 text-primary" />
        </div>
        {delta && (
          <span className="text-[10px] font-semibold text-emerald-400">{delta}</span>
        )}
      </div>
      <p className="mt-3 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
        {label}
      </p>
      <p className="mt-1 text-2xl font-bold">{value}</p>
    </Card>
  );
}

function OverviewTab() {
  const [chartLoading, setChartLoading] = useState(true);

  useEffect(() => {
    const t = setTimeout(() => setChartLoading(false), 800);
    return () => clearTimeout(t);
  }, []);

  const hasData = userGrowth7d.length > 0;
  const maxPop = Math.max(...popularServices.map((s) => s.count), 1);

  return (
    <div className="space-y-4">
      {/* ── Live Metrics ── */}
      <div className="grid grid-cols-2 gap-3">
        <KpiCard Icon={Users} label="Total Users" value={fmtNum(liveMetrics.totalUsers)} delta="+8.2%" />
        <KpiCard Icon={Activity} label="DAU" value={fmtNum(liveMetrics.dau)} delta="+2.4%" />
        <KpiCard Icon={TrendingUp} label="MAU" value={fmtNum(liveMetrics.mau)} delta="+5.1%" />
        <KpiCard Icon={Star} label="Donators" value={fmtNum(liveMetrics.donators)} delta="+12%" />
      </div>

      {/* ── User Growth Chart ── */}
      <Card className="border-white/10 bg-surface p-4">
        <div className="mb-4 flex items-center justify-between">
          <p className="text-sm font-semibold">User Growth</p>
          <span className="text-[10px] uppercase tracking-wider text-muted-foreground">
            Last 7 days
          </span>
        </div>

        {chartLoading ? (
          <ChartSkeleton />
        ) : !hasData ? (
          <ChartEmpty />
        ) : (
          <div className="h-52 w-full animate-in fade-in duration-500">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={userGrowth7d} margin={{ top: 8, right: 12, left: -8, bottom: 4 }}>
                <defs>
                  <linearGradient id="growthGradient" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="hsl(var(--primary))" stopOpacity={0.35} />
                    <stop offset="95%" stopColor="hsl(var(--primary))" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid stroke="hsl(var(--border))" strokeDasharray="4 4" opacity={0.15} vertical={false} />
                <XAxis dataKey="day" stroke="hsl(var(--muted-foreground))" fontSize={11} tickLine={false} axisLine={false} dy={6} />
                <YAxis stroke="hsl(var(--muted-foreground))" fontSize={10} tickLine={false} axisLine={false} tickFormatter={(v: number) => (v >= 1000 ? `${(v / 1000).toFixed(1)}k` : String(v))} width={40} />
                <Tooltip
                  contentStyle={{ background: "hsl(var(--surface-elevated))", border: "1px solid hsl(var(--border))", borderRadius: 12, fontSize: 12, padding: "8px 12px", boxShadow: "0 8px 24px rgba(0,0,0,.3)" }}
                  labelStyle={{ color: "hsl(var(--muted-foreground))", fontWeight: 600, marginBottom: 4 }}
                  itemStyle={{ color: "hsl(var(--foreground))" }}
                  formatter={(value: number) => [value.toLocaleString(), "Users"]}
                  cursor={{ stroke: "hsl(var(--primary))", strokeWidth: 1, strokeDasharray: "4 4" }}
                />
                <Area type="monotone" dataKey="users" stroke="hsl(var(--primary))" strokeWidth={2.5} fill="url(#growthGradient)" dot={{ r: 3.5, fill: "hsl(var(--primary))", stroke: "hsl(var(--background))", strokeWidth: 2 }} activeDot={{ r: 6, fill: "hsl(var(--primary))", stroke: "hsl(var(--background))", strokeWidth: 2.5 }} />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        )}
      </Card>

      {/* ── Popular Services Top-10 ── */}
      <Card className="border-white/10 bg-surface p-4">
        <div className="mb-4 flex items-center justify-between">
          <p className="text-sm font-semibold">Popular Services</p>
          <span className="text-[10px] uppercase tracking-wider text-muted-foreground">Top 10</span>
        </div>
        <div className="space-y-2.5">
          {popularServices.map((s, i) => (
            <div key={s.brand} className="flex items-center gap-3">
              <span className="w-4 text-right text-[10px] font-bold text-muted-foreground">{i + 1}</span>
              <div className="min-w-0 flex-1">
                <div className="mb-1 flex items-center justify-between">
                  <span className="truncate text-xs font-semibold">{s.name}</span>
                  <span className="ml-2 shrink-0 text-[11px] tabular-nums text-muted-foreground">{fmtNum(s.count)}</span>
                </div>
                <div className="bg-surface-elevated h-1.5 overflow-hidden rounded-full">
                  <div
                    className="bg-gradient-primary h-full rounded-full transition-all duration-500"
                    style={{ width: `${(s.count / maxPop) * 100}%` }}
                  />
                </div>
              </div>
            </div>
          ))}
        </div>
      </Card>
    </div>
  );
}

function ChartSkeleton() {
  return (
    <div className="flex h-52 w-full flex-col justify-end gap-2 px-2">
      <div className="flex items-end gap-3">
        <div className="flex w-8 flex-col items-end gap-6">
          <Skeleton className="h-2.5 w-6" />
          <Skeleton className="h-2.5 w-8" />
          <Skeleton className="h-2.5 w-5" />
          <Skeleton className="h-2.5 w-7" />
        </div>
        <div className="flex flex-1 items-end gap-1.5">
          {[40, 55, 50, 65, 60, 75, 70].map((h, i) => (
            <Skeleton key={i} className="flex-1 rounded-t-md" style={{ height: `${h}%` }} />
          ))}
        </div>
      </div>
      <div className="ml-11 flex justify-between">
        {Array.from({ length: 7 }).map((_, i) => (
          <Skeleton key={i} className="h-2.5 w-6" />
        ))}
      </div>
    </div>
  );
}

function ChartEmpty() {
  return (
    <div className="flex h-52 w-full flex-col items-center justify-center gap-3 text-muted-foreground">
      <BarChart3 className="h-10 w-10 opacity-30" />
      <p className="text-sm font-medium">No data yet</p>
      <p className="text-xs opacity-60">Growth data will appear here once users start signing up</p>
    </div>
  );
}

function CatalogTab() {
  const [items, setItems] = useState<AdminCatalogService[]>(adminCatalog);
  const [editing, setEditing] = useState<AdminCatalogService | null>(null);
  const [open, setOpen] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<AdminCatalogService | null>(null);

  const openAdd = () => {
    setEditing(null);
    setOpen(true);
  };
  const openEdit = (s: AdminCatalogService) => {
    setEditing(s);
    setOpen(true);
  };

  const handleSave = (data: Omit<AdminCatalogService, "id"> & { id?: string }) => {
    if (data.id) {
      setItems((prev) => prev.map((p) => (p.id === data.id ? { ...(p), ...data, id: data.id! } : p)));
      toast.success("Service updated");
    } else {
      const id = data.name.toLowerCase().replace(/\s+/g, "-").slice(0, 24) || Math.random().toString(36).slice(2);
      setItems((prev) => [...prev, { ...data, id }]);
      toast.success("Service added to catalog");
    }
    setOpen(false);
    setEditing(null);
  };

  const confirmDelete = () => {
    if (!pendingDelete) return;
    setItems((prev) => prev.filter((s) => s.id !== pendingDelete.id));
    toast.success(`${pendingDelete.name} removed`);
    setPendingDelete(null);
  };

  return (
    <div className="space-y-3">
      <Card className="border-white/10 bg-surface divide-y divide-white/5 p-0 overflow-hidden">
        {items.map((s) => (
          <div key={s.id} className="flex items-center gap-3 p-3">
            <img
              src={`https://logo.clearbit.com/${s.domain}`}
              alt={s.name}
              className="h-10 w-10 rounded-xl border border-white/10 bg-secondary object-cover"
              onError={(e) => {
                e.currentTarget.style.display = "none";
              }}
            />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <p className="truncate text-sm font-semibold">{s.name}</p>
                {!s.active && (
                  <span className="rounded-full bg-surface-elevated px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wide text-muted-foreground">
                    Off
                  </span>
                )}
              </div>
              <p className="text-[11px] text-muted-foreground">{s.category} • {s.domain}</p>
            </div>
            <button
              onClick={() => openEdit(s)}
              className="bg-surface-elevated flex h-8 w-8 items-center justify-center rounded-full transition-transform active:scale-90"
              aria-label="Edit"
            >
              <Edit3 className="h-3.5 w-3.5" />
            </button>
            <button
              onClick={() => setPendingDelete(s)}
              className="bg-surface-elevated hover:bg-destructive/20 hover:text-destructive flex h-8 w-8 items-center justify-center rounded-full transition-colors active:scale-90"
              aria-label="Delete"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          </div>
        ))}
        {items.length === 0 && (
          <p className="py-8 text-center text-xs text-muted-foreground">Catalog empty.</p>
        )}
      </Card>

      <Button
        onClick={openAdd}
        className="bg-gradient-primary text-white shadow-elevated sticky bottom-4 w-full rounded-2xl py-6 text-sm font-semibold"
      >
        <Plus className="mr-1 h-4 w-4" /> Add New Service
      </Button>

      <ServiceFormDialog
        open={open}
        onOpenChange={setOpen}
        initial={editing}
        onSave={handleSave}
        onDelete={
          editing
            ? () => {
                setOpen(false);
                setPendingDelete(editing);
              }
            : undefined
        }
      />

      <AlertDialog open={!!pendingDelete} onOpenChange={(o) => !o && setPendingDelete(null)}>
        <AlertDialogContent className="rounded-2xl">
          <AlertDialogHeader>
            <AlertDialogTitle>Delete service?</AlertDialogTitle>
            <AlertDialogDescription>
              <span className="font-medium">{pendingDelete?.name}</span> will be removed from the public catalog. Existing user subscriptions are unaffected.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={confirmDelete}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function ServiceFormDialog({
  open,
  onOpenChange,
  initial,
  onSave,
  onDelete,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  initial: AdminCatalogService | null;
  onSave: (data: Omit<AdminCatalogService, "id"> & { id?: string }) => void;
  onDelete?: () => void;
}) {
  const [name, setName] = useState("");
  const [category, setCategory] = useState<string>(SERVICE_CATEGORIES[0]);
  const [domain, setDomain] = useState("");
  const [active, setActive] = useState(true);

  // Reset on open / initial change
  const lastKey = `${open}-${initial?.id ?? "new"}`;
  const initRef = useRef<string>("");
  if (initRef.current !== lastKey) {
    initRef.current = lastKey;
    if (open) {
      setName(initial?.name ?? "");
      setCategory(initial?.category ?? SERVICE_CATEGORIES[0]);
      setDomain(initial?.domain ?? "");
      setActive(initial?.active ?? true);
    }
  }

  const submit = () => {
    if (!name.trim() || !domain.trim()) {
      toast.error("Name and domain are required.");
      return;
    }
    onSave({
      id: initial?.id,
      name: name.trim(),
      category,
      domain: domain.trim().replace(/^https?:\/\//, "").replace(/\/$/, ""),
      active,
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="rounded-2xl sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{initial ? "Edit Service" : "Add New Service"}</DialogTitle>
          <DialogDescription className="sr-only">Service catalog entry form</DialogDescription>
        </DialogHeader>

        <div className="space-y-4 pt-2">
          {domain && (
            <div className="flex items-center gap-3">
              <img
                src={`https://logo.clearbit.com/${domain.replace(/^https?:\/\//, "")}`}
                alt=""
                className="h-12 w-12 rounded-xl border border-white/10 bg-secondary object-cover"
                onError={(e) => {
                  e.currentTarget.style.display = "none";
                }}
              />
              <p className="text-xs text-muted-foreground">Logo preview from Clearbit</p>
            </div>
          )}

          <div className="space-y-1.5">
            <Label htmlFor="svc-name">Name</Label>
            <Input
              id="svc-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Netflix"
            />
          </div>

          <div className="space-y-1.5">
            <Label>Category</Label>
            <Select value={category} onValueChange={setCategory}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SERVICE_CATEGORIES.map((c) => (
                  <SelectItem key={c} value={c}>{c}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="svc-domain">Domain</Label>
            <Input
              id="svc-domain"
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              placeholder="netflix.com"
            />
          </div>

          <div className="bg-surface flex items-center justify-between rounded-xl p-3">
            <div>
              <p className="text-sm font-medium">Active</p>
              <p className="text-[11px] text-muted-foreground">
                Visible in the user catalog.
              </p>
            </div>
            <Switch checked={active} onCheckedChange={setActive} />
          </div>
        </div>

        <DialogFooter className="mt-2 flex-col gap-2 sm:flex-row sm:justify-between">
          {onDelete ? (
            <Button
              variant="ghost"
              onClick={onDelete}
              className="rounded-2xl text-destructive hover:text-destructive hover:bg-destructive/10 transition-transform active:scale-[0.98] sm:mr-auto"
            >
              <Trash2 className="mr-1 h-4 w-4" /> Delete
            </Button>
          ) : (
            <span />
          )}
          <div className="flex gap-2">
            <Button variant="outline" onClick={() => onOpenChange(false)} className="rounded-2xl transition-transform active:scale-[0.98]">
              Cancel
            </Button>
            <Button onClick={submit} className="bg-gradient-primary rounded-2xl text-white shadow-elevated transition-transform active:scale-[0.98]">
              {initial ? "Save" : "Add"}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function TrafficTab() {
  const [linkTag, setLinkTag] = useState("ad_channel");
  const generatedLink = `t.me/SubGuardBot?start=${linkTag.replace(/\s+/g, "_")}`;

  const copyLink = () => {
    navigator.clipboard.writeText(generatedLink);
    toast.success("Link copied!");
  };

  const maxFunnel = funnelSteps[0]?.value ?? 1;

  return (
    <div className="space-y-4">
      {/* ── Conversion Funnel ── */}
      <Card className="border-white/10 bg-surface p-4">
        <div className="mb-4 flex items-center justify-between">
          <p className="text-sm font-semibold">Conversion Funnel</p>
          <span className="text-[10px] uppercase tracking-wider text-muted-foreground">All time</span>
        </div>
        <div className="space-y-3">
          {funnelSteps.map((step, i) => {
            const pct = ((step.value / maxFunnel) * 100).toFixed(1);
            const convRate = i > 0
              ? `${((step.value / funnelSteps[i - 1].value) * 100).toFixed(1)}%`
              : null;
            return (
              <div key={step.label}>
                <div className="mb-1 flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-semibold">{step.label}</span>
                    {convRate && (
                      <span className="text-[10px] text-muted-foreground">→ {convRate}</span>
                    )}
                  </div>
                  <span className="text-xs font-bold tabular-nums">{fmtNum(step.value)}</span>
                </div>
                <div className="bg-surface-elevated h-3 overflow-hidden rounded-full">
                  <div
                    className="h-full rounded-full transition-all duration-700"
                    style={{ width: `${pct}%`, backgroundColor: step.color }}
                  />
                </div>
              </div>
            );
          })}
        </div>
      </Card>

      {/* ── Deep Link Generator ── */}
      <Card className="border-white/10 bg-surface p-4">
        <div className="mb-3 flex items-center gap-2">
          <Link2 className="h-4 w-4 text-primary" />
          <p className="text-sm font-semibold">Deep Link Generator</p>
        </div>
        <p className="mb-3 text-xs text-muted-foreground">
          Create tracked links for ad campaigns. Tag is appended as <code className="text-primary">?start=tag</code>.
        </p>
        <div className="flex gap-2">
          <Input
            value={linkTag}
            onChange={(e) => setLinkTag(e.target.value.replace(/[^a-zA-Z0-9_-]/g, ""))}
            placeholder="ad_campaign_name"
            className="bg-background/50 flex-1 font-mono text-xs"
          />
          <Button
            onClick={copyLink}
            className="bg-gradient-primary shrink-0 rounded-xl px-3 text-white transition-transform active:scale-[0.98]"
          >
            <Copy className="h-4 w-4" />
          </Button>
        </div>
        <div className="bg-surface-elevated mt-2 rounded-xl px-3 py-2">
          <p className="break-all font-mono text-[11px] text-muted-foreground">{generatedLink}</p>
        </div>
      </Card>

      {/* ── Deep Link Stats ── */}
      <Card className="border-white/10 bg-surface p-4">
        <div className="mb-3 flex items-center justify-between">
          <p className="text-sm font-semibold">Link Performance</p>
          <span className="text-[10px] uppercase tracking-wider text-muted-foreground">7 days</span>
        </div>
        <div className="space-y-2">
          {deepLinkStats.map((dl) => (
            <div key={dl.tag} className="bg-surface-elevated rounded-xl p-3">
              <div className="flex items-center justify-between">
                <p className="font-mono text-xs font-semibold text-primary">{dl.tag}</p>
                <span className="text-[10px] font-bold text-emerald-400">CR {dl.cr}</span>
              </div>
              <div className="mt-1.5 flex gap-4 text-[10px] text-muted-foreground">
                <span>Clicks <strong className="text-foreground">{fmtNum(dl.clicks)}</strong></span>
                <span>Starts <strong className="text-foreground">{fmtNum(dl.botStarts)}</strong></span>
                <span>Auths <strong className="text-foreground">{fmtNum(dl.auths)}</strong></span>
              </div>
            </div>
          ))}
        </div>
      </Card>
    </div>
  );
}

function BroadcastTab() {
  const [ru, setRu] = useState("");
  const [en, setEn] = useState("");
  const [image, setImage] = useState("");
  const [sending, setSending] = useState(false);

  const send = async () => {
    if (!ru.trim() && !en.trim()) {
      toast.error("Add at least one message body.");
      return;
    }
    setSending(true);
    await new Promise((r) => setTimeout(r, 1200));
    setSending(false);
    setRu("");
    setEn("");
    setImage("");
    toast.success("Broadcast queued", {
      description: `Sending to ${fmtNum(adminKpis.totalUsers)} users.`,
    });
  };

  return (
    <Card className="border-white/10 bg-surface space-y-4 p-4">
      <div className="space-y-1.5">
        <label className="text-xs font-semibold text-muted-foreground">Message (RU)</label>
        <Textarea
          value={ru}
          onChange={(e) => setRu(e.target.value)}
          placeholder="Привет! У нас новости..."
          className="bg-background/50 min-h-24"
        />
      </div>
      <div className="space-y-1.5">
        <label className="text-xs font-semibold text-muted-foreground">Message (EN)</label>
        <Textarea
          value={en}
          onChange={(e) => setEn(e.target.value)}
          placeholder="Hey! We have updates..."
          className="bg-background/50 min-h-24"
        />
      </div>
      <div className="space-y-1.5">
        <label className="text-xs font-semibold text-muted-foreground">Image URL</label>
        <Input
          value={image}
          onChange={(e) => setImage(e.target.value)}
          placeholder="https://…"
          className="bg-background/50"
        />
      </div>
      <Button
        onClick={send}
        disabled={sending}
        className="bg-gradient-primary text-white shadow-elevated w-full rounded-2xl py-6 text-sm font-semibold"
      >
        <Send className="mr-1 h-4 w-4" />
        {sending ? "Sending…" : "Send Broadcast"}
      </Button>
    </Card>
  );
}

function SettingsTab() {
  const [cpa, setCpa] = useState(adminGlobalSettings.cpaEnabled);
  const [gate, setGate] = useState(adminGlobalSettings.channelGateEnabled);
  const [channel, setChannel] = useState(adminGlobalSettings.targetChannel);

  const onChange = (label: string, val: boolean) => {
    toast.success(`${label} ${val ? "enabled" : "disabled"}`);
  };

  return (
    <div className="space-y-3">
      <Card className="border-white/10 bg-surface p-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-sm font-semibold">CPA Module (Native Ads)</p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              Show partner offers in the dashboard footer.
            </p>
          </div>
          <Switch
            checked={cpa}
            onCheckedChange={(v) => {
              setCpa(v);
              onChange("CPA module", v);
            }}
          />
        </div>
      </Card>

      <Card className="border-white/10 bg-surface p-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-sm font-semibold">Channel Soft-Gate</p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              Require users to join your Telegram channel before access.
            </p>
          </div>
          <Switch
            checked={gate}
            onCheckedChange={(v) => {
              setGate(v);
              onChange("Channel gate", v);
            }}
          />
        </div>
      </Card>

      <Card className="border-white/10 bg-surface p-4">
        <label className="text-xs font-semibold text-muted-foreground">
          Target Channel Username
        </label>
        <Input
          value={channel}
          onChange={(e) => setChannel(e.target.value)}
          placeholder="@mychannel"
          className="bg-background/50 mt-2"
        />
        <Button
          onClick={() => toast.success("Channel saved", { description: channel })}
          className="bg-gradient-primary text-white mt-3 w-full rounded-xl"
        >
          Save channel
        </Button>
      </Card>
    </div>
  );
}

export function AdminPanel() {
  return (
    <div className="bg-background min-h-screen pb-16">
      <header className="safe-top bg-background/85 sticky top-0 z-10 flex items-center justify-between border-b border-white/10 px-5 py-4 backdrop-blur-xl">
        <div>
          <p className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
            SubGuard
          </p>
          <h1 className="text-lg font-bold">Admin Control</h1>
        </div>
        <Link
          to="/"
          className="bg-surface-elevated flex items-center gap-1 rounded-full px-3 py-1.5 text-xs font-medium transition-transform active:scale-95"
        >
          <ChevronLeft className="h-3 w-3" />
          Back to App
        </Link>
      </header>

      <div className="px-5 pt-4">
        <Tabs defaultValue="overview" className="w-full">
          <TabsList className="bg-surface no-scrollbar mb-4 flex w-full justify-start gap-1 overflow-x-auto rounded-full p-1">
            {[
              { v: "overview", l: "Overview" },
              { v: "catalog", l: "Catalog" },
              { v: "traffic", l: "Traffic & CPA" },
              { v: "broadcast", l: "Broadcast" },
              { v: "settings", l: "Settings" },
            ].map((t) => (
              <TabsTrigger
                key={t.v}
                value={t.v}
                className="data-[state=active]:bg-gradient-primary data-[state=active]:text-white shrink-0 rounded-full px-4 py-1.5 text-xs font-semibold"
              >
                {t.l}
              </TabsTrigger>
            ))}
          </TabsList>

          <TabsContent value="overview"><OverviewTab /></TabsContent>
          <TabsContent value="catalog"><CatalogTab /></TabsContent>
          <TabsContent value="traffic"><TrafficTab /></TabsContent>
          <TabsContent value="broadcast"><BroadcastTab /></TabsContent>
          <TabsContent value="settings"><SettingsTab /></TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
