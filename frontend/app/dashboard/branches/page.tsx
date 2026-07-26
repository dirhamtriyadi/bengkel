"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation,useQueryClient } from "@tanstack/react-query";
import { Pencil,Plus } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { DataTable,type TableRow } from "@/components/data-table";
import { Button } from "@/components/ui/button";
import { Dialog,DialogContent,DialogDescription,DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { apiClient } from "@/lib/api";

const schema=z.object({code:z.string().min(1),name:z.string().min(1),address:z.string(),phone:z.string(),timezone:z.string().min(1),currency:z.string().length(3),is_active:z.boolean()});
type Form=z.infer<typeof schema>;
const defaults:Form={code:"",name:"",address:"",phone:"",timezone:"Asia/Jakarta",currency:"IDR",is_active:true};

export default function BranchesPage(){
 const client=useQueryClient();const [open,setOpen]=useState(false);const [editing,setEditing]=useState<TableRow|null>(null);const {register,handleSubmit,reset,formState:{errors}}=useForm<Form>({resolver:zodResolver(schema),defaultValues:defaults});
 const mutation=useMutation({mutationFn:(values:Form)=>apiClient(editing?`/branches/${editing.id}`:"/branches",{method:editing?"PUT":"POST",body:JSON.stringify(values)}),onSuccess:()=>{client.invalidateQueries({queryKey:["/branches"]});setOpen(false)}});
 function show(row?:TableRow){setEditing(row??null);reset(row?{code:String(row.code),name:String(row.name),address:String(row.address??""),phone:String(row.phone??""),timezone:String(row.timezone),currency:String(row.currency),is_active:Boolean(row.is_active)}:defaults);setOpen(true)}
 return <div className="space-y-6"><div><h1 className="text-3xl font-bold tracking-tight">Cabang</h1><p className="mt-2 text-muted-foreground">Master cabang untuk isolasi transaksi, stok, pengguna, dan pembukuan.</p></div><DataTable endpoint="/branches" defaultSort="code" columns={[{key:"code",label:"Kode"},{key:"name",label:"Nama"},{key:"address",label:"Alamat"},{key:"phone",label:"Telepon"},{key:"timezone",label:"Zona waktu"},{key:"is_active",label:"Status",format:value=>value?"Aktif":"Nonaktif"}]} toolbar={<Button onClick={()=>show()}><Plus className="size-4"/>Tambah cabang</Button>} actions={row=><Button variant="ghost" size="icon" onClick={()=>show(row)}><Pencil className="size-4"/></Button>}/><Dialog open={open} onOpenChange={setOpen}><DialogContent><DialogTitle>{editing?"Edit cabang":"Tambah cabang"}</DialogTitle><DialogDescription>Cabang nonaktif tetap menyimpan histori transaksi.</DialogDescription><form className="mt-6 grid gap-4 sm:grid-cols-2" onSubmit={handleSubmit(values=>mutation.mutate(values))}>{(["code","name","phone","timezone","currency"] as const).map(name=><label key={name} className="text-sm font-medium capitalize">{name.replace("_"," ")}<Input className="mt-2" {...register(name)}/>{errors[name]&&<span className="text-xs text-red-600">{errors[name]?.message}</span>}</label>)}<label className="text-sm font-medium sm:col-span-2">Alamat<textarea className="mt-2 min-h-24 w-full rounded-xl border p-3 text-sm" {...register("address")}/></label><label className="flex items-center gap-2 text-sm sm:col-span-2"><input type="checkbox" {...register("is_active")}/>Cabang aktif</label>{mutation.error&&<p className="text-sm text-red-600 sm:col-span-2">{mutation.error.message}</p>}<div className="flex justify-end gap-2 sm:col-span-2"><Button type="button" variant="outline" onClick={()=>setOpen(false)}>Batal</Button><Button disabled={mutation.isPending}>Simpan</Button></div></form></DialogContent></Dialog></div>
}
