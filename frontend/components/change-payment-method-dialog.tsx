"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useQueryClient } from "@tanstack/react-query";
import { CreditCard, Loader2, RefreshCw } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { apiClient } from "@/lib/api";
import { loadMidtransSnap, type MidtransSnapConfiguration } from "@/lib/midtrans";
import { rupiah } from "@/lib/utils";

const schema = z.object({
  method: z.enum(["cash", "midtrans"]),
  amount_received: z.coerce.number().int().min(0, "Uang diterima tidak boleh negatif"),
});

type Form = z.infer<typeof schema>;

export type ChangeablePayment = {
  id: string;
  sale_id: string;
  method: string;
  status: string;
  base_amount: number;
  amount: number;
  sale_number?: string;
};

type ChangeResult = {
  sale: { id: string; status: string };
  payment: ChangeablePayment;
  print_url: string;
};

type SnapResult = MidtransSnapConfiguration & { token: string };

export function ChangePaymentMethodDialog({ payment }: { payment: ChangeablePayment }) {
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [providerMessage, setProviderMessage] = useState("");
  const { register, handleSubmit, watch, reset, setError, formState: { errors } } = useForm<Form>({
    resolver: zodResolver(schema),
    defaultValues: { method: payment.method === "midtrans" ? "midtrans" : "cash", amount_received: payment.base_amount || payment.amount },
  });
  const method = watch("method");
  const baseAmount = payment.base_amount || payment.amount;

  function changeOpen(value: boolean) {
    setOpen(value);
    setProviderMessage("");
    if (value) {
      reset({ method: payment.method === "midtrans" ? "midtrans" : "cash", amount_received: baseAmount });
    }
  }

  async function refreshData() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["receipt", payment.sale_id] }),
      queryClient.invalidateQueries({ queryKey: ["/payments"] }),
      queryClient.invalidateQueries({ queryKey: ["/sales"] }),
    ]);
  }

  async function synchronize(paymentID: string, message: string) {
    setProviderMessage(message);
    try {
      await apiClient(`/payments/${paymentID}/midtrans/sync`, { method: "POST" });
    } finally {
      await refreshData();
      setOpen(false);
    }
  }

  async function submit(values: Form) {
    if (values.method === "cash" && values.amount_received < baseAmount) {
      setError("amount_received", { message: `Minimal ${rupiah.format(baseAmount)}` });
      return;
    }
    setBusy(true);
    setProviderMessage("");
    try {
      const changed = await apiClient<ChangeResult>(`/payments/${payment.id}/change-method`, {
        method: "POST",
        body: JSON.stringify(values),
      });
      if (!changed.data) throw new Error("Respons pergantian pembayaran tidak lengkap");
      if (values.method === "cash") {
        await refreshData();
        setOpen(false);
        return;
      }

      const newPaymentID = changed.data.payment.id;
      const snap = await apiClient<SnapResult>(`/payments/${newPaymentID}/midtrans/snap`, { method: "POST" });
      if (!snap.data?.token) throw new Error("Token Midtrans baru tidak tersedia");
      await loadMidtransSnap(snap.data);
      if (!window.snap) throw new Error("Midtrans Snap belum siap");
      window.snap.pay(snap.data.token, {
        onSuccess: () => { void synchronize(newPaymentID, "Pembayaran diterima dan sedang diverifikasi."); },
        onPending: () => { void synchronize(newPaymentID, "Instruksi pembayaran baru sudah dibuat."); },
        onError: () => { setProviderMessage("Pembayaran baru ditolak Midtrans. Attempt baru tetap tersimpan dan dapat diganti kembali."); void refreshData(); },
        onClose: () => { setProviderMessage("Jendela Midtrans ditutup. Attempt baru tersimpan dan dapat dilanjutkan dari invoice."); void refreshData(); },
      });
    } catch (caught) {
      setProviderMessage(caught instanceof Error ? caught.message : "Metode pembayaran gagal diganti");
      await refreshData();
    } finally {
      setBusy(false);
    }
  }

  return <>
    <Button size="sm" variant="outline" onClick={() => changeOpen(true)}>
      <RefreshCw className="size-4" />Ganti metode
    </Button>
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent className="max-w-lg">
        <DialogTitle>Ganti metode pembayaran</DialogTitle>
        <DialogDescription>
          Attempt lama akan dibatalkan dan tetap disimpan sebagai riwayat. Invoice {payment.sale_number || "ini"} tidak dibuat ulang.
        </DialogDescription>
        <form className="mt-6 space-y-5" onSubmit={handleSubmit(submit)}>
          <div className="rounded-xl border bg-muted/30 p-4 text-sm">
            <div className="flex justify-between gap-4"><span>Total tagihan</span><strong>{rupiah.format(baseAmount)}</strong></div>
            <div className="mt-2 flex justify-between gap-4"><span>Metode saat ini</span><strong className="uppercase">{payment.method}</strong></div>
          </div>
          <label className="block text-sm font-medium">
            Metode pengganti
            <select className="mt-2 h-10 w-full rounded-xl border bg-white px-3" {...register("method")}>
              <option value="midtrans">Midtrans — buat sesi baru</option>
              <option value="cash">Tunai</option>
            </select>
          </label>
          {method === "cash" && <label className="block text-sm font-medium">
            Uang diterima
            <Input className="mt-2" type="number" min={baseAmount} step={1} {...register("amount_received", { valueAsNumber: true })} />
            {errors.amount_received && <span className="mt-1 block text-xs text-red-600">{errors.amount_received.message}</span>}
          </label>}
          {method === "midtrans" && <div className="flex gap-3 rounded-xl border border-blue-200 bg-blue-50 p-4 text-sm text-blue-800">
            <CreditCard className="mt-0.5 size-5 shrink-0" />
            <p>Sesi Snap lama dibatalkan. Sistem menggunakan konfigurasi Midtrans terbaru dan membuat order reference baru.</p>
          </div>}
          {providerMessage && <p className="rounded-xl bg-orange-50 p-3 text-sm text-orange-800">{providerMessage}</p>}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" disabled={busy} onClick={() => changeOpen(false)}>Batal</Button>
            <Button disabled={busy}>{busy ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}{busy ? "Memproses..." : "Ganti metode"}</Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  </>;
}
