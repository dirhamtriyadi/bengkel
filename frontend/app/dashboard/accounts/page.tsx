"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation,useQuery,useQueryClient } from "@tanstack/react-query";
import { Pencil,Plus,Trash2 } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { DataTable,type TableRow } from "@/components/data-table";
import { Button } from "@/components/ui/button";
import { Dialog,DialogContent,DialogDescription,DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { apiClient } from "@/lib/api";

const schema=z.object({code:z.string().min(1,"Kode wajib diisi").max(30),name:z.string().min(1,"Nama wajib diisi").max(150),type:z.enum(["asset","liability","equity","revenue","expense"]),parent_id:z.string(),is_active:z.boolean()});
type Form=z.infer<typeof schema>;
const labels:Record<string,string>={asset:"Aset",liability:"Liabilitas",equity:"Ekuitas",revenue:"Pendapatan",expense:"Beban"};
type AccountOption={id:string;code:string;name:string};

export default function AccountsPage(){
  const client=useQueryClient();const [open,setOpen]=useState(false);const [editing,setEditing]=useState<TableRow|null>(null);
  const options=useQuery({queryKey:["account-parent-options"],queryFn:()=>apiClient<AccountOption[]>("/accounts?per_page=100&sort=code&direction=asc")});
  const {register,handleSubmit,reset,formState:{errors}}=useForm<Form>({resolver:zodResolver(schema),defaultValues:{code:"",name:"",type:"asset",parent_id:"",is_active:true}});
  const mutation=useMutation({mutationFn:(values:Form)=>apiClient(editing?`/accounts/${editing.id}`:"/accounts",{method:editing?"PUT":"POST",body:JSON.stringify({...values,parent_id:values.parent_id||null})}),onSuccess:()=>{client.invalidateQueries({queryKey:["/accounts"]});client.invalidateQueries({queryKey:["account-parent-options"]});setOpen(false);setEditing(null);reset()},});
  function create(){setEditing(null);reset({code:"",name:"",type:"asset",parent_id:"",is_active:true});setOpen(true)}
  function edit(row:TableRow){setEditing(row);reset({code:String(row.code),name:String(row.name),type:row.type as Form["type"],parent_id:String(row.parent_id??""),is_active:Boolean(row.is_active)});setOpen(true)}
  async function remove(row:TableRow){if(!window.confirm(`Hapus akun ${row.code}?`))return;await apiClient(`/accounts/${row.id}`,{method:"DELETE"});client.invalidateQueries({queryKey:["/accounts"]})}
  return <div className="space-y-6">
    <div><h1 className="text-3xl font-bold tracking-tight">Chart of accounts</h1><p className="mt-2 text-muted-foreground">Kelola kode akun per cabang untuk jurnal double-entry.</p></div>
    <DataTable endpoint="/accounts" defaultSort="code" columns={[{key:"code",label:"Kode"},{key:"name",label:"Nama akun"},{key:"type",label:"Klasifikasi",format:value=>labels[String(value)]??String(value)},{key:"is_active",label:"Status",format:value=>value?"Aktif":"Nonaktif"}]} toolbar={<Button onClick={create}><Plus className="size-4"/>Tambah akun</Button>} actions={row=><div className="flex gap-1"><Button variant="ghost" size="icon" onClick={()=>edit(row)} title="Edit"><Pencil className="size-4"/></Button><Button variant="ghost" size="icon" onClick={()=>remove(row)} title="Hapus"><Trash2 className="size-4 text-red-600"/></Button></div>}/>
    <Dialog open={open} onOpenChange={setOpen}><DialogContent><DialogTitle>{editing?"Edit akun":"Tambah akun"}</DialogTitle><DialogDescription>Kode akun harus unik dalam cabang aktif.</DialogDescription><form className="mt-6 space-y-4" onSubmit={handleSubmit(values=>mutation.mutate(values))}><Field label="Kode" error={errors.code?.message}><Input {...register("code")}/></Field><Field label="Nama akun" error={errors.name?.message}><Input {...register("name")}/></Field><Field label="Klasifikasi" error={errors.type?.message}><select className="h-10 w-full rounded-xl border bg-white px-3 text-sm" {...register("type")}>{Object.entries(labels).map(([value,label])=><option key={value} value={value}>{label}</option>)}</select></Field><Field label="Akun induk"><select className="h-10 w-full rounded-xl border bg-white px-3 text-sm" {...register("parent_id")}><option value="">Tanpa akun induk</option>{options.data?.data?.filter(account=>account.id!==editing?.id).map(account=><option key={account.id} value={account.id}>{account.code} — {account.name}</option>)}</select></Field><label className="flex items-center gap-2 text-sm"><input type="checkbox" {...register("is_active")}/>Akun aktif</label>{mutation.error&&<p className="rounded-xl bg-red-50 p-3 text-sm text-red-700">{mutation.error.message}</p>}<div className="flex justify-end gap-2"><Button type="button" variant="outline" onClick={()=>setOpen(false)}>Batal</Button><Button disabled={mutation.isPending}>{mutation.isPending?"Menyimpan...":"Simpan"}</Button></div></form></DialogContent></Dialog>
  </div>
}

function Field({label,error,children}:{label:string;error?:string;children:React.ReactNode}){return <label className="block text-sm font-medium">{label}<div className="mt-2">{children}</div>{error&&<span className="mt-1 block text-xs text-red-600">{error}</span>}</label>}
