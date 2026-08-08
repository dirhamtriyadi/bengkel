"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Code2, CreditCard, Globe2, Save } from "lucide-react";
import { useEffect, useState } from "react";
import { apiClient } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { WhatsAppIntegrationCard } from "@/components/whatsapp-integration-card";
import { MidtransIntegrationCard } from "@/components/midtrans-integration-card";

type MidtransChannel = {
  payment_type: string;
  acquirer?: string;
  label: string;
  enabled: boolean;
  customer_percentage: number;
  fee_percentage: number;
  fixed_fee: number;
  tax_percentage: number;
};
type MidtransFeeSettings = { automatic_fee: boolean; channels: MidtransChannel[] };
type Setting = { id: string; key: string; value: unknown; is_public: boolean };
type CMSPage = {
  id: string;
  slug: string;
  title: string;
  meta_title: string;
  meta_description: string;
  status: "draft" | "published" | "archived";
  content: Record<string, unknown>;
};

const defaultFees: MidtransFeeSettings = {
  automatic_fee: true,
  channels: [
    { payment_type: "bca_va", label: "BCA Virtual Account", enabled: true, customer_percentage: 100, fee_percentage: 0, fixed_fee: 4000, tax_percentage: 11 },
    { payment_type: "bni_va", label: "BNI Virtual Account", enabled: true, customer_percentage: 100, fee_percentage: 0, fixed_fee: 4000, tax_percentage: 11 },
    { payment_type: "bri_va", label: "BRI Virtual Account", enabled: true, customer_percentage: 100, fee_percentage: 0, fixed_fee: 4000, tax_percentage: 11 },
    { payment_type: "permata_va", label: "Permata Virtual Account", enabled: true, customer_percentage: 100, fee_percentage: 0, fixed_fee: 4000, tax_percentage: 11 },
    { payment_type: "echannel", label: "Mandiri Bill Payment", enabled: true, customer_percentage: 100, fee_percentage: 0, fixed_fee: 4000, tax_percentage: 11 },
    { payment_type: "gopay", label: "GoPay", enabled: true, customer_percentage: 100, fee_percentage: 2, fixed_fee: 0, tax_percentage: 11 },
    { payment_type: "qris", acquirer: "gopay", label: "QRIS", enabled: true, customer_percentage: 100, fee_percentage: 0.7, fixed_fee: 0, tax_percentage: 0 },
    { payment_type: "shopeepay", label: "ShopeePay", enabled: true, customer_percentage: 100, fee_percentage: 2, fixed_fee: 0, tax_percentage: 11 },
    { payment_type: "credit_card", label: "Kartu kredit", enabled: true, customer_percentage: 100, fee_percentage: 2.9, fixed_fee: 2000, tax_percentage: 11 },
    { payment_type: "indomaret", label: "Indomaret", enabled: true, customer_percentage: 100, fee_percentage: 0, fixed_fee: 1000, tax_percentage: 0 },
    { payment_type: "alfamart", label: "Alfamart / Alfamidi / DAN+DAN", enabled: true, customer_percentage: 100, fee_percentage: 0, fixed_fee: 5000, tax_percentage: 0 },
    { payment_type: "akulaku", label: "Akulaku PayLater", enabled: true, customer_percentage: 100, fee_percentage: 1.7, fixed_fee: 0, tax_percentage: 11 },
  ],
};

function isFeeSettings(value: unknown): value is MidtransFeeSettings {
  return typeof value === "object" && value !== null && "channels" in value && Array.isArray(value.channels);
}

export default function SettingsPage() {
  const client = useQueryClient();
  const settings = useQuery({ queryKey: ["settings"], queryFn: () => apiClient<Setting[]>("/settings") });
  const pages = useQuery({ queryKey: ["cms-pages"], queryFn: () => apiClient<CMSPage[]>("/cms/pages?per_page=10") });
  const home = pages.data?.data?.find((page) => page.slug === "home");
  const configured = settings.data?.data?.find((row) => row.key === "payment.midtrans.channels");
  const [fees, setFees] = useState<MidtransFeeSettings>(defaultFees);
  const [cms, setCMS] = useState("");
  const [metaTitle, setMetaTitle] = useState("");
  const [metaDescription, setMetaDescription] = useState("");
  const [message, setMessage] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (isFeeSettings(configured?.value)) setFees(configured.value);
  }, [configured]);
  useEffect(() => {
    if (home) {
      setCMS(JSON.stringify(home.content, null, 2));
      setMetaTitle(home.meta_title);
      setMetaDescription(home.meta_description);
    }
  }, [home]);

  function updateChannel(index: number, values: Partial<MidtransChannel>) {
    setFees((current) => ({
      ...current,
      channels: current.channels.map((channel, channelIndex) => channelIndex === index ? { ...channel, ...values } : channel),
    }));
  }

  async function saveFee() {
    setSaving(true);
    setMessage("");
    try {
      await apiClient("/settings/payment.midtrans.channels", {
        method: "PUT",
        body: JSON.stringify({ value: fees, is_public: false }),
      });
      await client.invalidateQueries({ queryKey: ["settings"] });
      setMessage("Konfigurasi biaya per channel Midtrans tersimpan.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Gagal menyimpan pengaturan.");
    } finally {
      setSaving(false);
    }
  }

  async function saveCMS() {
    if (!home) return;
    setSaving(true);
    setMessage("");
    try {
      const content = JSON.parse(cms) as Record<string, unknown>;
      await apiClient("/cms/pages/home", {
        method: "PUT",
        body: JSON.stringify({ title: home.title, meta_title: metaTitle, meta_description: metaDescription, content, status: home.status }),
      });
      await client.invalidateQueries({ queryKey: ["cms-pages"] });
      setMessage("Landing page tersimpan. Perubahan publik mengikuti cache maksimal 60 detik.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "JSON CMS tidak valid.");
    } finally {
      setSaving(false);
    }
  }

  return <div className="space-y-6">
    <div>
      <h1 className="text-3xl font-bold tracking-tight">Pengaturan</h1>
      <p className="mt-2 text-muted-foreground">Konfigurasi cabang, pembayaran, dan landing page.</p>
    </div>
    {message && <p className="rounded-xl border bg-white p-4 text-sm">{message}</p>}

    <MidtransIntegrationCard />

    <WhatsAppIntegrationCard />

    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2"><CreditCard className="size-5" />Biaya payment gateway per channel</CardTitle>
        <CardDescription>
          Porsi pelanggan dikirim ke Automatic Fee Imposition Midtrans. Nilai 100% berarti seluruh fee channel dibayar pelanggan, 0% berarti seluruhnya ditanggung bengkel.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <label className="flex items-start gap-3 rounded-xl border p-4 text-sm">
          <input
            className="mt-1 size-4"
            type="checkbox"
            checked={fees.automatic_fee}
            onChange={(event) => setFees((current) => ({ ...current, automatic_fee: event.target.checked }))}
          />
          <span>
            <strong className="block">Gunakan perhitungan otomatis Midtrans</strong>
            <span className="text-muted-foreground">Direkomendasikan agar nominal mengikuti MDR dan biaya transaksi yang berlaku pada akun Midtrans.</span>
          </span>
        </label>
        <div className="overflow-x-auto rounded-xl border">
          <table className="min-w-[980px] w-full text-sm">
            <thead className="bg-muted/70 text-left">
              <tr>
                <th className="p-3">Aktif</th>
                <th className="p-3">Channel</th>
                <th className="p-3">Porsi pelanggan</th>
                <th className="p-3">Referensi MDR</th>
                <th className="p-3">Biaya tetap</th>
                <th className="p-3">Pajak fee</th>
              </tr>
            </thead>
            <tbody>
              {fees.channels.map((channel, index) => <tr className="border-t" key={channel.payment_type}>
                <td className="p-3">
                  <input type="checkbox" className="size-4" checked={channel.enabled} onChange={(event) => updateChannel(index, { enabled: event.target.checked })} />
                </td>
                <td className="p-3">
                  <p className="font-semibold">{channel.label}</p>
                  <code className="text-xs text-muted-foreground">{channel.payment_type}{channel.acquirer ? ` · ${channel.acquirer}` : ""}</code>
                </td>
                <td className="p-3">
                  <div className="flex items-center gap-2"><Input className="w-24" type="number" min={0} max={100} step={0.01} value={channel.customer_percentage} onChange={(event) => updateChannel(index, { customer_percentage: Number(event.target.value) })} /><span>%</span></div>
                </td>
                <td className="p-3">
                  <div className="flex items-center gap-2"><Input className="w-24" type="number" min={0} max={100} step={0.01} value={channel.fee_percentage} onChange={(event) => updateChannel(index, { fee_percentage: Number(event.target.value) })} /><span>%</span></div>
                </td>
                <td className="p-3">
                  <Input className="w-32" type="number" min={0} step={1} value={channel.fixed_fee} onChange={(event) => updateChannel(index, { fixed_fee: Number(event.target.value) })} />
                </td>
                <td className="p-3">
                  <div className="flex items-center gap-2"><Input className="w-24" type="number" min={0} max={100} step={0.01} value={channel.tax_percentage} onChange={(event) => updateChannel(index, { tax_percentage: Number(event.target.value) })} /><span>%</span></div>
                </td>
              </tr>)}
            </tbody>
          </table>
        </div>
        <p className="text-xs text-muted-foreground">
          Referensi MDR, biaya tetap, dan pajak dipakai untuk rekonsiliasi beban merchant. Nominal yang ditagihkan ke pelanggan tetap memakai hasil aktual Midtrans saat channel dipilih.
        </p>
        <Button disabled={saving} onClick={saveFee}><Save className="size-4" />Simpan konfigurasi channel</Button>
      </CardContent>
    </Card>

    <div className="grid gap-5 lg:grid-cols-2">
      <Card>
        <CardHeader><CardTitle className="flex items-center gap-2"><Globe2 className="size-5" />SEO landing page</CardTitle><CardDescription>Metadata SEO halaman utama dari CMS.</CardDescription></CardHeader>
        <CardContent className="space-y-3">
          <label className="text-sm font-medium">Meta title<Input className="mt-2" maxLength={200} value={metaTitle} onChange={(event) => setMetaTitle(event.target.value)} /></label>
          <label className="text-sm font-medium">Meta description<Input className="mt-2" maxLength={320} value={metaDescription} onChange={(event) => setMetaDescription(event.target.value)} /></label>
        </CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle>Konfigurasi aktif</CardTitle></CardHeader>
        <CardContent className="space-y-3">{settings.data?.data?.map((row) => <div key={row.id} className="rounded-xl border p-4"><p className="font-mono text-sm font-semibold">{row.key}</p><pre className="mt-1 overflow-hidden text-xs text-muted-foreground">{JSON.stringify(row.value)}</pre></div>)}</CardContent>
      </Card>
    </div>

    <Card>
      <CardHeader><CardTitle className="flex items-center gap-2"><Code2 className="size-5" />Blok konten landing page</CardTitle><CardDescription>Edit struktur hero dan features. Payload disimpan sebagai JSONB dan dirender server-side untuk SEO.</CardDescription></CardHeader>
      <CardContent>
        <textarea className="min-h-80 w-full rounded-xl border bg-slate-950 p-4 font-mono text-sm text-slate-100 outline-none focus:ring-2 focus:ring-primary/30" value={cms} onChange={(event) => setCMS(event.target.value)} spellCheck={false} />
        <Button className="mt-4" disabled={saving || !home} onClick={saveCMS}><Save className="size-4" />Publikasikan konten</Button>
      </CardContent>
    </Card>
  </div>;
}
