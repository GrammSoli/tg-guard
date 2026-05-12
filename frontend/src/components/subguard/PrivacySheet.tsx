import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  ChevronRight,
  Download,
  FileText,
  ShieldAlert,
  Trash2,
} from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
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
import { api, ApiError } from "@/lib/api";
import { closeMiniApp, hapticImpact, openExternalLink } from "@/lib/telegram";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// Placeholder URLs — replace with real telegra.ph posts once they're up.
const PRIVACY_POLICY_URL = "https://telegra.ph/SubGuard-Privacy-Policy";
const TERMS_OF_SERVICE_URL = "https://telegra.ph/SubGuard-Terms-of-Service";

/**
 * PrivacySheet — three sections:
 *   1. "Your data"      — download a GDPR-style JSON dump
 *   2. "Documents"      — links to privacy policy + ToS (open in-Telegram)
 *   3. "Danger zone"    — account deletion, gated behind AlertDialog
 *
 * The delete flow does NOT do its own optimistic state. Once the API call
 * succeeds we close the WebApp (or, in a browser, hard-reload). There's no
 * recovery path — that's the point.
 */
export function PrivacySheet({ open, onOpenChange }: Props) {
  const { t } = useTranslation();

  const [downloading, setDownloading] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const handleDownload = async () => {
    if (downloading) return;
    hapticImpact("light");
    setDownloading(true);
    try {
      // We let api() parse the response as JSON. The Content-Disposition
      // header from the backend is ignored — we re-package the data as a
      // Blob and trigger the download client-side. Simpler & works inside
      // Telegram WebView, which can't always handle direct attachment links.
      const data = await api<Record<string, unknown>>("/me/export");
      const blob = new Blob([JSON.stringify(data, null, 2)], {
        type: "application/json",
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      const stamp = new Date().toISOString().slice(0, 10);
      a.download = `subguard-export-${stamp}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
      toast.success(t("privacy.exportSuccess"));
    } catch (err) {
      const reason =
        err instanceof ApiError ? err.message : (err as Error)?.message ?? "unknown";
      toast.error(t("privacy.exportFailed", { reason }));
    } finally {
      setDownloading(false);
    }
  };

  const handleOpenPolicy = () => {
    hapticImpact("light");
    openExternalLink(PRIVACY_POLICY_URL);
  };

  const handleOpenTerms = () => {
    hapticImpact("light");
    openExternalLink(TERMS_OF_SERVICE_URL);
  };

  const handleDeleteRequest = () => {
    hapticImpact("medium");
    setConfirmOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (deleting) return;
    setDeleting(true);
    try {
      await api("/me", { method: "DELETE" });
      toast.success(t("privacy.deleteSuccess"));
      // Try graceful Telegram close first. If we're not inside Telegram
      // (e.g. dev preview in a browser) close() no-ops, so fall back to a
      // full reload after a short pause so any in-flight requests don't see
      // a still-logged-in state.
      closeMiniApp();
      setTimeout(() => {
        if (typeof window !== "undefined") window.location.href = "/";
      }, 800);
    } catch (err) {
      const reason =
        err instanceof ApiError ? err.message : (err as Error)?.message ?? "unknown";
      toast.error(t("privacy.deleteFailed", { reason }));
      setDeleting(false);
      setConfirmOpen(false);
    }
  };

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side="bottom" className="rounded-t-3xl">
          <SheetHeader className="text-left">
            <SheetTitle>{t("privacy.sheetTitle")}</SheetTitle>
            <SheetDescription>{t("privacy.sheetDesc")}</SheetDescription>
          </SheetHeader>

          <div className="mt-4 space-y-5 pb-6">
            {/* ── Your data ─────────────────────────────────── */}
            <section>
              <p className="mb-2 px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                {t("privacy.sectionData")}
              </p>
              <button
                onClick={handleDownload}
                disabled={downloading}
                className="bg-surface flex w-full items-center gap-3 rounded-2xl p-4 text-left transition-colors active:scale-[0.99] disabled:cursor-not-allowed disabled:opacity-50"
              >
                <div className="bg-surface-elevated flex h-10 w-10 items-center justify-center rounded-xl">
                  <Download className="h-4 w-4" />
                </div>
                <div className="flex-1">
                  <p className="text-sm font-semibold">
                    {t("privacy.downloadLabel")}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t("privacy.downloadHint")}
                  </p>
                </div>
                <ChevronRight className="h-4 w-4 text-muted-foreground" />
              </button>
            </section>

            {/* ── Documents ─────────────────────────────────── */}
            <section>
              <p className="mb-2 px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                {t("privacy.sectionDocuments")}
              </p>
              <div className="space-y-2">
                <button
                  onClick={handleOpenPolicy}
                  className="bg-surface flex w-full items-center gap-3 rounded-2xl p-4 text-left transition-colors active:scale-[0.99]"
                >
                  <div className="bg-surface-elevated flex h-10 w-10 items-center justify-center rounded-xl">
                    <FileText className="h-4 w-4" />
                  </div>
                  <p className="flex-1 text-sm font-semibold">
                    {t("privacy.policy")}
                  </p>
                  <ChevronRight className="h-4 w-4 text-muted-foreground" />
                </button>
                <button
                  onClick={handleOpenTerms}
                  className="bg-surface flex w-full items-center gap-3 rounded-2xl p-4 text-left transition-colors active:scale-[0.99]"
                >
                  <div className="bg-surface-elevated flex h-10 w-10 items-center justify-center rounded-xl">
                    <FileText className="h-4 w-4" />
                  </div>
                  <p className="flex-1 text-sm font-semibold">
                    {t("privacy.terms")}
                  </p>
                  <ChevronRight className="h-4 w-4 text-muted-foreground" />
                </button>
              </div>
            </section>

            {/* ── Danger zone ───────────────────────────────── */}
            <section>
              <p className="mb-2 flex items-center gap-1 px-1 text-[11px] font-semibold uppercase tracking-wider text-destructive">
                <ShieldAlert className="h-3 w-3" /> {t("privacy.sectionDanger")}
              </p>
              <button
                onClick={handleDeleteRequest}
                disabled={deleting}
                className="flex w-full items-center gap-3 rounded-2xl border border-destructive/30 bg-destructive/10 p-4 text-left transition-colors active:scale-[0.99] disabled:cursor-not-allowed disabled:opacity-50"
              >
                <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-destructive/20">
                  <Trash2 className="h-4 w-4 text-destructive" />
                </div>
                <div className="flex-1">
                  <p className="text-sm font-semibold text-destructive">
                    {t("privacy.deleteLabel")}
                  </p>
                  <p className="text-xs text-destructive/70">
                    {t("privacy.deleteHint")}
                  </p>
                </div>
              </button>
            </section>
          </div>
        </SheetContent>
      </Sheet>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent className="rounded-2xl">
          <AlertDialogHeader>
            <AlertDialogTitle>{t("privacy.confirmDeleteTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("privacy.confirmDeleteDesc")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>
              {t("privacy.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={deleting}
              onClick={(e) => {
                e.preventDefault();
                handleDeleteConfirm();
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? t("privacy.deleting") : t("privacy.confirmDelete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
