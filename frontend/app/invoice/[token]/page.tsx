"use client";

import { useQuery } from "@tanstack/react-query";
import { AlertCircle, CheckCircle2, Clock3, CreditCard, Printer, ReceiptText, ShieldCheck } from "lucide-react";
import { use, useState } from "react";
import { Button } from "@/components/ui/button";
import { publicApiClient } from "@/lib/api";
import { loadMidtransSnap, type MidtransSnapConfiguration } from "@/lib/midtrans";
import { dateTime, rupiah } from "@/lib/utils";

type PublicInvoice = {
  invoice: {
    number: string;
    status: string;
    created_at: string;
    subtotal: number;
    discount: number;
    tax: number;
    gateway_fee: number;
    grand_total: number;
    amount_paid: number;
  };
  customer: { name: string };
  branch: { name: string; address: string; phone: string };
  payment: {
    method: string;
    status: string;
    base_amount: number;
    amount: number;
    customer_fee: number;
    fee_bearer: string;
    payment_channel: string;
    environment: "sandbox" | "production";
    payable: boolean;
  };
  items: Array<{ description: string; type: string; quantity: number; unit_price: number; discount: number; subtotal: number }>;
  expires_at: string;
};

type SnapResult = MidtransSnapConfiguration & { token: string; redirect_url?: string; environment: string };

export default function PublicInvoicePage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = use(params);
  const [paying, setPaying] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const encodedToken = encodeURIComponent(token);
  const query = useQuery({
    queryKey: ["public-invoice", token],
    queryFn: () => publicApiClient<PublicInvoice>(`/invoices/${encodedToken}`),
    retry: false,
    refetchInterval: (result) => result.state.data?.data?.invoice.status === "pending" ? 5000 : false,
  });
  const data = query.data?.data;

  async function syncPayment(message: string) {
    setNotice(message);
    setError("");
    try {
      await publicApiClient(`/invoices/${encodedToken}/midtrans/sync`, { method: "POST" });
      await query.refetch();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Status pembayaran belum dapat diperbarui");
    } finally {
      setPaying(false);
    }
  }

  async function pay() {
    setPaying(true);
    setError("");
    setNotice("");
    try {
      const result = await publicApiClient<SnapResult>(`/invoices/${encodedToken}/midtrans/snap`, { method: "POST" });
      if (!result.data?.token) {
        throw new Error("Halaman pembayaran Midtrans belum siap. Muat ulang lalu coba lagi.");
      }
      await loadMidtransSnap(result.data);
      if (!window.snap) throw new Error("Halaman pembayaran Midtrans belum siap.");
      window.snap.pay(result.data.token, {
        onSuccess: () => { void syncPayment("Pembayaran diterima. Status invoice sedang diverifikasi."); },
        onPending: () => { void syncPayment("Instruksi pembayaran sudah dibuat. Selesaikan pembayaran sesuai channel yang dipilih."); },
        onError: () => { setPaying(false); setError("Pembayaran ditolak oleh Midtrans. Silakan coba kembali."); },
        onClose: () => { setPaying(false); setNotice("Jendela pembayaran ditutup. Invoice masih dapat dibayar selama belum kedaluwarsa."); },
      });
    } catch (caught) {
      setPaying(false);
      setError(caught instanceof Error ? caught.message : "Pembayaran tidak dapat dimulai");
    }
  }

  if (query.isLoading) {
    return <main className="grid min-h-screen place-items-center p-6"><div className="text-center"><ReceiptText className="mx-auto size-10 animate-pulse text-primary"/><p className="mt-4 text-sm text-muted-foreground">Memuat invoice...</p></div></main>;
  }
  if (!data) {
    return <main className="grid min-h-screen place-items-center p-6"><section className="max-w-md rounded-2xl border bg-white p-8 text-center shadow-sm"><AlertCircle className="mx-auto size-12 text-destructive"/><h1 className="mt-4 text-xl font-bold">Invoice tidak tersedia</h1><p className="mt-2 text-sm text-muted-foreground">{query.error instanceof Error ? query.error.message : "Tautan salah, sudah dicabut, atau telah kedaluwarsa."}</p></section></main>;
  }

  const paid = data.invoice.status === "paid" || data.payment.status === "paid";
  const failed = ["void", "failed", "cancelled"].includes(data.invoice.status) || data.payment.status === "failed";
  return <main className="receipt-page receipt-a4 mx-auto min-h-screen bg-white p-5 sm:p-10">
    <div className="print-hidden mb-6 flex items-center justify-between gap-3 rounded-2xl border bg-background p-4">
      <div className="flex items-center gap-3"><ShieldCheck className="size-6 text-emerald-600"/><div><p className="text-sm font-bold">Tautan pembayaran aman</p><p className="text-xs text-muted-foreground">Jangan bagikan tautan invoice ini.</p></div></div>
      <Button variant="outline" size="sm" onClick={() => window.print()}><Printer className="size-4"/>Cetak</Button>
    </div>
    <section className="mx-auto max-w-[760px]">
      <header className="flex flex-col justify-between gap-5 border-b-2 border-foreground pb-6 sm:flex-row">
        <div><p className="text-xs font-bold uppercase tracking-[.2em] text-primary">Invoice pembayaran</p><h1 className="mt-2 text-2xl font-black">{data.branch.name}</h1><p className="mt-2 max-w-sm text-sm text-muted-foreground">{data.branch.address}<br/>{data.branch.phone}</p></div>
        <div className="sm:text-right"><p className="font-black">{data.invoice.number}</p><p className="mt-1 text-sm text-muted-foreground">{dateTime.format(new Date(data.invoice.created_at))}</p><p className="mt-2 text-sm">Untuk <strong>{data.customer.name || "Pelanggan"}</strong></p></div>
      </header>

      <div className={`my-5 flex items-start gap-3 rounded-xl border p-4 ${paid ? "border-emerald-200 bg-emerald-50 text-emerald-800" : failed ? "border-red-200 bg-red-50 text-red-800" : "border-orange-200 bg-orange-50 text-orange-800"}`}>
        {paid ? <CheckCircle2 className="mt-0.5 size-5 shrink-0"/> : <Clock3 className="mt-0.5 size-5 shrink-0"/>}
        <div><p className="font-bold">{paid ? "Pembayaran berhasil" : failed ? "Invoice tidak dapat dibayar" : "Menunggu pembayaran"}</p><p className="mt-1 text-xs">{paid ? "Pembayaran telah terverifikasi dan invoice ini sah sebagai bukti transaksi." : failed ? "Status transaksi sudah berakhir. Hubungi bengkel bila membutuhkan invoice baru." : `Tautan berlaku sampai ${dateTime.format(new Date(data.expires_at))}.`}</p></div>
      </div>

      {notice && <p className="print-hidden mb-4 rounded-xl bg-blue-50 p-4 text-sm text-blue-700">{notice}</p>}
      {error && <p className="print-hidden mb-4 rounded-xl bg-red-50 p-4 text-sm text-red-700">{error}</p>}

      <div className="overflow-x-auto"><table className="my-6 w-full min-w-[560px] text-sm"><thead><tr className="border-b text-left"><th className="py-3">Item</th><th className="py-3 text-right">Qty</th><th className="py-3 text-right">Harga</th><th className="py-3 text-right">Diskon</th><th className="py-3 text-right">Jumlah</th></tr></thead><tbody>{data.items.map((item, index) => <tr key={`${item.description}-${index}`} className="border-b"><td className="py-3"><p className="font-semibold">{item.description}</p><p className="text-xs uppercase text-muted-foreground">{item.type}</p></td><td className="py-3 text-right">{item.quantity}</td><td className="py-3 text-right">{rupiah.format(item.unit_price)}</td><td className="py-3 text-right">{rupiah.format(item.discount)}</td><td className="py-3 text-right font-semibold">{rupiah.format(item.subtotal)}</td></tr>)}</tbody></table></div>

      <div className="ml-auto max-w-sm space-y-2 text-sm">
        <div className="flex justify-between"><span>Subtotal</span><span>{rupiah.format(data.invoice.subtotal)}</span></div>
        <div className="flex justify-between"><span>Diskon</span><span>-{rupiah.format(data.invoice.discount)}</span></div>
        <div className="flex justify-between"><span>Pajak</span><span>{rupiah.format(data.invoice.tax)}</span></div>
        <div className="flex justify-between"><span>Biaya admin pelanggan</span><span>{rupiah.format(data.invoice.gateway_fee || data.payment.customer_fee)}</span></div>
        <div className="flex justify-between border-t-2 border-foreground pt-3 text-lg font-black"><span>Total</span><span>{rupiah.format(data.invoice.grand_total)}</span></div>
        {data.payment.payment_channel && <div className="flex justify-between text-xs text-muted-foreground"><span>Channel</span><span className="uppercase">{data.payment.payment_channel.replaceAll("_", " ")}</span></div>}
      </div>

      {data.payment.payable && <div className="print-hidden mt-8 rounded-2xl border bg-background p-5"><div className="flex items-start gap-3"><CreditCard className="mt-0.5 size-5 text-primary"/><div><p className="font-bold">Bayar online dengan Midtrans</p><p className="mt-1 text-sm text-muted-foreground">Pilih channel di halaman Midtrans. Nominal akhir termasuk biaya admin pelanggan akan dihitung sesuai channel dan pengaturan bengkel.</p></div></div><Button size="lg" className="mt-5 w-full" onClick={pay} disabled={paying}>{paying ? "Menyiapkan pembayaran..." : "Lanjut ke pembayaran"}</Button></div>}

      <footer className="mt-10 border-t pt-5 text-center text-xs text-muted-foreground"><p className="font-semibold text-foreground">{data.payment.method === "midtrans" ? "Pembayaran online diproses oleh Midtrans." : "Pembayaran tunai dicatat oleh bengkel."}</p><p className="mt-1">Hubungi {data.branch.name} jika detail invoice tidak sesuai.</p></footer>
    </section>
  </main>;
}
