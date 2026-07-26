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

const schema=z.object({code:z.string().min(1).max(60),name:z.string().min(1).max(100),description:z.string(),permission_ids:z.array(z.string().uuid()).min(1,"Pilih minimal satu permission")});
type Form=z.infer<typeof schema>;type Permission={id:string;code:string;name:string};

export default function RolesPage(){
 const client=useQueryClient();const [open,setOpen]=useState(false);const [editing,setEditing]=useState<TableRow|null>(null);const permissions=useQuery({queryKey:["permission-options"],queryFn:()=>apiClient<Permission[]>("/permissions")});
 const {register,handleSubmit,reset,watch,setValue,formState:{errors}}=useForm<Form>({resolver:zodResolver(schema),defaultValues:{code:"",name:"",description:"",permission_ids:[]}});const selected=watch("permission_ids");
 const mutation=useMutation({mutationFn:(values:Form)=>apiClient(editing?`/roles/${editing.id}`:"/roles",{method:editing?"PUT":"POST",body:JSON.stringify(values)}),onSuccess:()=>{client.invalidateQueries({queryKey:["/roles"]});client.invalidateQueries({queryKey:["roles-options"]});setOpen(false)}});
 function show(row?:TableRow){setEditing(row??null);reset({code:String(row?.code??""),name:String(row?.name??""),description:String(row?.description??""),permission_ids:((row?.permissions as Permission[]|undefined)??[]).map(value=>value.id)});setOpen(true)}
 function toggle(id:string){setValue("permission_ids",selected.includes(id)?selected.filter(value=>value!==id):[...selected,id],{shouldValidate:true})}
 async function remove(row:TableRow){if(!window.confirm(`Hapus role ${row.name}?`))return;await apiClient(`/roles/${row.id}`,{method:"DELETE"});client.invalidateQueries({queryKey:["/roles"]})}
 const groups=(permissions.data?.data??[]).reduce<Record<string,Permission[]>>((result,permission)=>{const key=permission.code.split(".")[0];(result[key]??=[]).push(permission);return result},{});
 return <div className="space-y-6"><div><h1 className="text-3xl font-bold tracking-tight">Role & permission</h1><p className="mt-2 text-muted-foreground">RBAC granular untuk membatasi menu dan aksi API.</p></div><DataTable endpoint="/roles" defaultSort="name" columns={[{key:"code",label:"Kode"},{key:"name",label:"Nama"},{key:"description",label:"Deskripsi"},{key:"permissions",label:"Permission",format:value=>`${(value as Permission[]??[]).length} permission`}]} toolbar={<Button onClick={()=>show()}><Plus className="size-4"/>Tambah role</Button>} actions={row=>row.code==="owner"?<span className="text-xs text-muted-foreground">Dilindungi</span>:<div className="flex gap-1"><Button variant="ghost" size="icon" onClick={()=>show(row)}><Pencil className="size-4"/></Button><Button variant="ghost" size="icon" onClick={()=>remove(row)}><Trash2 className="size-4 text-red-600"/></Button></div>}/><Dialog open={open} onOpenChange={setOpen}><DialogContent className="max-w-3xl"><DialogTitle>{editing?"Edit role":"Tambah role"}</DialogTitle><DialogDescription>Permission yang dipilih menjadi sumber otorisasi API dan visibilitas navigasi.</DialogDescription><form className="mt-6 space-y-4" onSubmit={handleSubmit(values=>mutation.mutate(values))}><div className="grid gap-4 sm:grid-cols-2"><Field label="Kode" error={errors.code?.message}><Input {...register("code")}/></Field><Field label="Nama" error={errors.name?.message}><Input {...register("name")}/></Field></div><Field label="Deskripsi"><Input {...register("description")}/></Field><div className="max-h-80 space-y-4 overflow-y-auto rounded-xl border p-4">{Object.entries(groups).map(([group,items])=><fieldset key={group}><legend className="mb-2 font-semibold capitalize">{group.replaceAll("_"," ")}</legend><div className="grid gap-2 sm:grid-cols-2">{items?.map(permission=><label key={permission.id} className="flex items-center gap-2 rounded-lg bg-muted/40 p-2 text-sm"><input type="checkbox" checked={selected.includes(permission.id)} onChange={()=>toggle(permission.id)}/>{permission.code}</label>)}</div></fieldset>)}</div>{errors.permission_ids&&<p className="text-xs text-red-600">{errors.permission_ids.message}</p>}{mutation.error&&<p className="text-sm text-red-600">{mutation.error.message}</p>}<div className="flex justify-end gap-2"><Button type="button" variant="outline" onClick={()=>setOpen(false)}>Batal</Button><Button disabled={mutation.isPending}>Simpan</Button></div></form></DialogContent></Dialog></div>
}
function Field({label,error,children}:{label:string;error?:string;children:React.ReactNode}){return <label className="text-sm font-medium">{label}<div className="mt-2">{children}</div>{error&&<span className="text-xs text-red-600">{error}</span>}</label>}
