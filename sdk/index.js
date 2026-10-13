/** Server-side client. Keep analytics API keys out of browser bundles. */
export class ClickShieldAnalytics {
 constructor({baseURL,apiKey,fetch:fetcher=globalThis.fetch}) {
  const u=new URL(baseURL);if(!['http:','https:'].includes(u.protocol)||u.username||u.password)throw new TypeError('HTTP(S) baseURL required');
  if(typeof apiKey!=='string'||!/^(cst|csak)_/.test(apiKey))throw new TypeError('ClickShield API token required');
  this.baseURL=u.toString().replace(/\/$/,'');this.apiKey=apiKey;this.fetch=fetcher;
 }
 async request(path,filters={},options={}) {
  if(typeof path!=='string'||!path.startsWith('/')||path.startsWith('//'))throw new TypeError('Relative API path required');
  const url=new URL(this.baseURL+path);
  for(const [k,v] of Object.entries(filters))if(v!==undefined&&v!==null){if(Array.isArray(v)){for(const item of v)url.searchParams.append(k,String(item))}else url.searchParams.set(k,v instanceof Date?v.toISOString():String(v))}
  const response=await this.fetch(url,{method:'GET',headers:{Authorization:`Bearer ${this.apiKey}`,Accept:'application/json'},signal:options.signal,redirect:'error'});
  const result=await response.json();if(!response.ok){const e=new Error(result.error||`HTTP ${response.status}`);e.status=response.status;throw e};return result;
 }
 report(report,filters={},options={}){return this.request('/v1/reports',{...filters,report},options)}
 timeline(report,filters={},options={}){return this.request('/v1/reports/timeline',{...filters,report},options)}
 async behavior(kind,filters={},options={}){const allowed=['events','properties','goals','funnels','journey','retention','heatmap'];if(!allowed.includes(kind))throw new TypeError('Unknown behavior report');return (await this.request(`/v1/analytics/${kind}`,filters,options)).data}
 async definitions(projectID,options={}){return (await this.request(`/v1/projects/${encodeURIComponent(projectID)}/definitions`,{},options)).definitions}
 async tools(projectID,kind,filters={},options={}){if(!['links','pixels','annotations'].includes(kind))throw new TypeError('Unknown analytics tool');return (await this.request(`/v1/projects/${encodeURIComponent(projectID)}/tools/${kind}`,filters,options)).data}
}
