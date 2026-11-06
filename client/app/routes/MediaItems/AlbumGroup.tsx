import type { LoaderFunctionArgs } from "react-router";
import Album from "./Album";
export async function clientLoader({params,request}: LoaderFunctionArgs) {
 const res=await fetch(`/apis/web/v1/album-group/${params.id}`,{signal:request.signal});
 if(!res.ok) throw new Response("Album group could not be found", {status:res.status});
 return res.json();
}
export default Album;
