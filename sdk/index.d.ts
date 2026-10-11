export interface Filters {
 project_id?:string;from?:string|Date;to?:string|Date;timezone?:string;
 classification?:'HUMAN'|'SUSPICIOUS'|'BOT'|'KNOWN_BOT'|'UNCLASSIFIED'|'NO_BOTS';
 channel?:string;source?:string;campaign?:string;country?:string;region?:string;city?:string;browser?:string;os?:string;language?:string;device?:string;
 event?:string;page_url?:string;width?:number;interval?:'hour'|'day'|'week'|'month';series?:string|string[];total_only?:boolean;
}
export interface Options {signal?:AbortSignal}
export interface Metrics {visits:number;visitors:number;new_visitors:number;pageviews:number;bounces:number;humans:number;suspicious:number;bots:number;known_bots:number;leads:number;qualified:number;converted_visits:number;qualified_visits:number;replays:number;average_duration_ms:number}
export interface Report {kind:string;dimensions:string[];rows:(Metrics&{dimensions:string[];level:number})[];totals:Metrics;timezone:string}
export interface Step {type:'page'|'event'|'outcome';value:string;match?:'exact'|'prefix';properties?:Record<string,string|number|boolean|null>}
export interface Definition {id:string;project_id:string;kind:'goal'|'funnel';name:string;steps:Step[];window_minutes:number}
export interface EventMetric {name:string;events:number;visits:number;visitors:number}
export interface PropertyMetric {property:string;value:string|number|boolean|null;events:number}
export interface Funnel {definition:Definition;eligible_visits:number;counts:number[]}
export interface JourneyEdge {from:string;to:string;transitions:number;visits:number}
export interface RetentionCell {cohort:string;day:number;visitors:number;returned:number;observed:boolean}
export interface Heatmap {page_url:string;width:number;visits:number;pages:{url:string;width:number;visits:number}[];clicks:{x:number;y:number;count:number}[];scroll:{depth:number;visits:number}[]}
export interface BehaviorReports {events:EventMetric[];properties:PropertyMetric[];goals:Funnel[];funnels:Funnel[];journey:JourneyEdge[];retention:RetentionCell[];heatmap:Heatmap}
export interface Timeline {interval:string;timezone:string;buckets:string[];total:Metrics[];series:{key:string;points:Metrics[]}[]}
export interface TrackedLink {id:string;project_id:string;slug:string;name:string;destination:string;enabled:boolean;hits:number}
export interface TrackingPixel {id:string;project_id:string;name:string;enabled:boolean;hits:number}
export interface Annotation {id:string;project_id:string;at:string;text:string}
export interface ToolReports {links:TrackedLink[];pixels:TrackingPixel[];annotations:Annotation[]}
export class ClickShieldAnalytics {
 constructor(options:{baseURL:string;apiKey:string;fetch?:typeof fetch});
 report(report:string,filters?:Filters,options?:Options):Promise<Report>;
 timeline(report:string,filters?:Filters,options?:Options):Promise<Timeline>;
 behavior<K extends keyof BehaviorReports>(kind:K,filters?:Filters,options?:Options):Promise<BehaviorReports[K]>;
 definitions(projectID:string,options?:Options):Promise<Definition[]>;
 tools<K extends keyof ToolReports>(projectID:string,kind:K,filters?:Filters,options?:Options):Promise<ToolReports[K]>;
}
declare global {interface Window {ClickShield?:{track(name:string,properties?:Record<string,string|number|boolean|null>):boolean;sessionId:string;visitorId:string;pageIndex():number;flush():Promise<void>;getDecision():string}}}
