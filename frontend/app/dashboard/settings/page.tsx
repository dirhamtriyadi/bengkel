"use client";

import { useQuery,useQueryClient } from "@tanstack/react-query";
import { Code2,CreditCard,Globe2,Save } from "lucide-react";
import { useEffect,useState } from "react";
import { apiClient } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card,CardContent,CardDescription,CardHeader,CardTitle } from "@/components/ui/card";

type Setting={id:string;key:string;value:{value?:string;split_percentage?:number}|unknown;is_public:boolean};
type CMSPage={id:string;slug:string;title:string;meta_title:string;meta_description:string;status:"draft"|"published"|"archived";content:Record<string,unknown>};

export default function SettingsPage(){
  const client=useQueryClient();
  const settings=useQuery({queryKey:["settings"],queryFn:()=>apiClient<Setting[]>("/settings")});
  const pages=useQuery({queryKey:["cms-pages"],queryFn:()=>apiClient<CMSPage[]>("/cms/pages?per_page=10")});
  const home=pages.data?.data?.find(page=>page.slug==="home");
  const configured=settings.data?.data?.find(row=>row.key==="payment.midtrans.fee_bearer");
  const configuredValue=typeof configured?.value==="object"&&configured.value!==null&&"value" in configured.value?String(configured.value.value):"customer";
  const [feeBearer,setFeeBearer]=useState("customer");
  const [cms,setCMS]=useState("");
  const [metaTitle,setMetaTitle]=useState("");
  const [metaDescription,setMetaDescription]=useState("");
  const [message,setMessage]=useState("");
  const [saving,setSaving]=useState(false);

  useEffect(()=>{setFeeBearer(configuredValue)},[configuredValue]);
  useEffect(()=>{if(home){setCMS(JSON.stringify(home.content,null,2));setMetaTitle(home.meta_title);setMetaDescription(home.meta_description)}},[home]);

  async function saveFee(){
    setSaving(true);setMessage("");
    try{await apiClient("/settings/payment.midtrans.fee_bearer",{method:"PUT",body:JSON.stringify({value:{value:feeBearer,split_percentage:50},is_public:false})});await client.invalidateQueries({queryKey:["settings"]});setMessage("Pengaturan payment gateway tersimpan.")}
    catch(error){setMessage(error instanceof Error?error.message:"Gagal menyimpan pengaturan.")}
    finally{setSaving(false)}
  }

  async function saveCMS(){
    if(!home)return;
    setSaving(true);setMessage("");
    try{const content=JSON.parse(cms) as Record<string,unknown>;await apiClient("/cms/pages/home",{method:"PUT",body:JSON.stringify({title:home.title,meta_title:metaTitle,meta_description:metaDescription,content,status:home.status})});await client.invalidateQueries({queryKey:["cms-pages"]});setMessage("Landing page tersimpan. Perubahan publik mengikuti cache maksimal 60 detik.")}
    catch(error){setMessage(error instanceof Error?error.message:"JSON CMS tidak valid.")}
    finally{setSaving(false)}
  }

  return <div className="space-y-6">
    <div><h1 className="text-3xl font-bold tracking-tight">Pengaturan</h1><p className="mt-2 text-muted-foreground">Konfigurasi cabang, pembayaran, dan landing page.</p></div>
    {message&&<p className="rounded-xl border bg-white p-4 text-sm">{message}</p>}
    <div className="grid gap-5 lg:grid-cols-2">
      <Card><CardHeader><CardTitle className="flex items-center gap-2"><CreditCard className="size-5"/>Payment gateway</CardTitle><CardDescription>Penanggung biaya Midtrans dapat diubah per cabang.</CardDescription></CardHeader><CardContent><label className="text-sm font-medium">Penanggung biaya<select className="mt-2 h-10 w-full rounded-xl border bg-white px-3" value={feeBearer} onChange={event=>setFeeBearer(event.target.value)}><option value="customer">Pelanggan</option><option value="merchant">Bengkel</option><option value="split">Dibagi 50:50</option></select></label><Button className="mt-5" disabled={saving} onClick={saveFee}><Save className="size-4"/>Simpan</Button></CardContent></Card>
      <Card><CardHeader><CardTitle className="flex items-center gap-2"><Globe2 className="size-5"/>SEO landing page</CardTitle><CardDescription>Metadata SEO halaman utama dari CMS.</CardDescription></CardHeader><CardContent className="space-y-3"><label className="text-sm font-medium">Meta title<Input className="mt-2" maxLength={200} value={metaTitle} onChange={event=>setMetaTitle(event.target.value)}/></label><label className="text-sm font-medium">Meta description<Input className="mt-2" maxLength={320} value={metaDescription} onChange={event=>setMetaDescription(event.target.value)}/></label></CardContent></Card>
    </div>
    <Card><CardHeader><CardTitle className="flex items-center gap-2"><Code2 className="size-5"/>Blok konten landing page</CardTitle><CardDescription>Edit struktur hero dan features. Payload disimpan sebagai JSONB dan dirender server-side untuk SEO.</CardDescription></CardHeader><CardContent><textarea className="min-h-80 w-full rounded-xl border bg-slate-950 p-4 font-mono text-sm text-slate-100 outline-none focus:ring-2 focus:ring-primary/30" value={cms} onChange={event=>setCMS(event.target.value)} spellCheck={false}/><Button className="mt-4" disabled={saving||!home} onClick={saveCMS}><Save className="size-4"/>Publikasikan konten</Button></CardContent></Card>
    <Card><CardHeader><CardTitle>Konfigurasi aktif</CardTitle></CardHeader><CardContent className="space-y-3">{settings.data?.data?.map(row=><div key={row.id} className="flex items-center justify-between rounded-xl border p-4"><div><p className="font-mono text-sm font-semibold">{row.key}</p><pre className="mt-1 max-w-xl overflow-hidden text-xs text-muted-foreground">{JSON.stringify(row.value)}</pre></div><span className="text-xs">{row.is_public?"Publik":"Privat"}</span></div>)}</CardContent></Card>
  </div>
}
