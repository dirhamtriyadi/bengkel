"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, CreditCard, Loader2, Save, ShieldCheck } from "lucide-react";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { apiClient } from "@/lib/api";

type Integration = {
  enabled: boolean;
  environment: "sandbox" | "production";
  merchant_id: string;
  server_key_configured: boolean;
  client_key_configured: boolean;
};

const schema = z.object({
  enabled: z.boolean(),
  environment: z.enum(["sandbox", "production"]),
  merchant_id: z.string().trim().max(100),
  server_key: z.string().trim().max(2048),
  client_key: z.string().trim().max(2048),
  clear_server_key: z.boolean(),
  clear_client_key: z.boolean(),
}).superRefine((value, context) => {
  if (value.server_key) {
    const prefix = value.environment === "sandbox" ? "SB-Mid-server-" : "Mid-server-";
    if (!value.server_key.startsWith(prefix)) context.addIssue({ code: z.ZodIssueCode.custom, path: ["server_key"], message: `Server Key ${value.environment} harus berawalan ${prefix}` });
  }
  if (value.client_key) {
    const prefix = value.environment === "sandbox" ? "SB-Mid-client-" : "Mid-client-";
    if (!value.client_key.startsWith(prefix)) context.addIssue({ code: z.ZodIssueCode.custom, path: ["client_key"], message: `Client Key ${value.environment} harus berawalan ${prefix}` });
  }
});
type Form = z.infer<typeof schema>;

export function MidtransIntegrationCard() {
  const queryClient = useQueryClient();
  const [notice, setNotice] = useState("");
  const [actionError, setActionError] = useState("");
  const configuration = useQuery({ queryKey: ["midtrans-integration"], queryFn: () => apiClient<Integration>("/integrations/midtrans") });
  const integration = configuration.data?.data;
  const { register, handleSubmit, reset, setError, watch, formState: { errors, isSubmitting } } = useForm<Form>({
    resolver: zodResolver(schema),
    defaultValues: { enabled: false, environment: "sandbox", merchant_id: "", server_key: "", client_key: "", clear_server_key: false, clear_client_key: false },
  });
  const environment = watch("environment");

  useEffect(() => {
    if (!integration) return;
    reset({
      enabled: integration.enabled,
      environment: integration.environment,
      merchant_id: integration.merchant_id,
      server_key: "",
      client_key: "",
      clear_server_key: false,
      clear_client_key: false,
    });
  }, [integration, reset]);

  async function save(values: Form) {
    setNotice("");
    setActionError("");
    const serverAvailable = Boolean(values.server_key || (integration?.server_key_configured && !values.clear_server_key));
    const clientAvailable = Boolean(values.client_key || (integration?.client_key_configured && !values.clear_client_key));
    if (values.enabled && !serverAvailable) {
      setError("server_key", { message: "Server Key wajib diisi saat integrasi diaktifkan" });
      return;
    }
    if (values.enabled && !clientAvailable) {
      setError("client_key", { message: "Client Key wajib diisi saat integrasi diaktifkan" });
      return;
    }
    try {
      await apiClient<Integration>("/integrations/midtrans", { method: "PUT", body: JSON.stringify(values) });
      await queryClient.invalidateQueries({ queryKey: ["midtrans-integration"] });
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
      setNotice("Konfigurasi Midtrans tersimpan terenkripsi di database.");
    } catch (caught) {
      setActionError(caught instanceof Error ? caught.message : "Konfigurasi Midtrans gagal disimpan");
    }
  }

  return <Card>
    <CardHeader>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <CardTitle className="flex items-center gap-2"><CreditCard className="size-5"/>Kredensial Midtrans</CardTitle>
          <CardDescription className="mt-2">Key disimpan terenkripsi per cabang di database. Server Key tidak pernah dikirim ke browser; Client Key hanya diberikan saat memuat Snap.js.</CardDescription>
        </div>
        <Badge className={integration?.enabled ? "bg-emerald-100 text-emerald-700" : "bg-slate-100 text-slate-700"}>{integration?.enabled ? `Aktif · ${integration.environment}` : "Tidak aktif"}</Badge>
      </div>
    </CardHeader>
    <CardContent>
      <form className="grid gap-4 lg:grid-cols-2" onSubmit={handleSubmit(save)}>
        <label className="flex items-start gap-3 rounded-xl border p-4 text-sm lg:col-span-2">
          <input type="checkbox" className="mt-1 size-4" {...register("enabled")}/>
          <span><strong className="block">Aktifkan pembayaran Midtrans</strong><span className="text-muted-foreground">Checkout Midtrans baru ditolak sampai konfigurasi lengkap dan integrasi diaktifkan.</span></span>
        </label>
        <label className="text-sm font-medium">Mode
          <select className="mt-2 h-10 w-full rounded-xl border bg-white px-3" {...register("environment")}>
            <option value="sandbox">Sandbox</option>
            <option value="production">Production</option>
          </select>
          <span className="mt-1 block text-xs text-muted-foreground">Pergantian mode memerlukan pasangan key untuk mode yang sama.</span>
        </label>
        <label className="text-sm font-medium">Merchant ID (opsional)<Input className="mt-2" placeholder="G123456789" {...register("merchant_id")}/>{errors.merchant_id&&<span className="mt-1 block text-xs text-destructive">{errors.merchant_id.message}</span>}</label>
        <label className="text-sm font-medium">Server Key
          <Input className="mt-2" type="password" autoComplete="new-password" placeholder={integration?.server_key_configured ? "Key sudah tersimpan — kosongkan untuk mempertahankan" : environment === "sandbox" ? "SB-Mid-server-..." : "Mid-server-..."} {...register("server_key")}/>
          {integration?.server_key_configured&&<span className="mt-1 flex items-center gap-1 text-xs text-emerald-700"><CheckCircle2 className="size-3"/>Server Key terenkripsi sudah tersimpan</span>}
          {errors.server_key&&<span className="mt-1 block text-xs text-destructive">{errors.server_key.message}</span>}
          {integration?.server_key_configured&&<label className="mt-2 flex items-center gap-2 text-xs font-normal text-muted-foreground"><input type="checkbox" {...register("clear_server_key")}/>Hapus Server Key tersimpan</label>}
        </label>
        <label className="text-sm font-medium">Client Key
          <Input className="mt-2" type="password" autoComplete="new-password" placeholder={integration?.client_key_configured ? "Key sudah tersimpan — kosongkan untuk mempertahankan" : environment === "sandbox" ? "SB-Mid-client-..." : "Mid-client-..."} {...register("client_key")}/>
          {integration?.client_key_configured&&<span className="mt-1 flex items-center gap-1 text-xs text-emerald-700"><CheckCircle2 className="size-3"/>Client Key terenkripsi sudah tersimpan</span>}
          {errors.client_key&&<span className="mt-1 block text-xs text-destructive">{errors.client_key.message}</span>}
          {integration?.client_key_configured&&<label className="mt-2 flex items-center gap-2 text-xs font-normal text-muted-foreground"><input type="checkbox" {...register("clear_client_key")}/>Hapus Client Key tersimpan</label>}
        </label>
        <div className="rounded-xl bg-blue-50 p-4 text-xs text-blue-800 lg:col-span-2"><ShieldCheck className="mb-2 size-5"/><strong>Transaksi pending tetap konsisten.</strong> Setiap checkout menyimpan snapshot kredensial terenkripsi, sehingga rotasi key berikutnya tidak mengubah verifikasi transaksi lama.</div>
        <div className="lg:col-span-2"><Button disabled={isSubmitting}>{isSubmitting?<Loader2 className="size-4 animate-spin"/>:<Save className="size-4"/>}{isSubmitting?"Menyimpan...":"Simpan konfigurasi Midtrans"}</Button></div>
      </form>
      {(notice||actionError)&&<p className={`mt-4 rounded-xl p-3 text-sm ${actionError?"bg-red-50 text-red-700":"bg-emerald-50 text-emerald-700"}`}>{actionError||notice}</p>}
    </CardContent>
  </Card>;
}
