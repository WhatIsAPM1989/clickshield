import {storage, PALETTE} from "./analytics-common.js";
import {DIMS, REPORT_DIMS} from "./analytics-model.js";

// ---------- State ----------
const state={
  csrf:'',report:'sources',data:null,prev:null,prevIndex:new Map(),timeline:null,prevTimeline:null,
  filters:new Map(),segment:'',compare:false,interval:'auto',chartMetric:'visits',chartType:storage.get('chart-type','line'),
  selected:[],hidden:new Set(),colors:new Map(),sort:{key:'visits',dir:-1},expanded:new Set(),expandAll:false,
  search:'',page:0,groups:new Set(storage.get('column-groups',['traffic','behavior','quality','goals'])),
  request:0,timelineRequest:0,metrikaOffset:1,metrikaRequest:0,
};


function colorFor(key){
  if(!state.colors.has(key)){const used=new Set(state.colors.values());state.colors.set(key,PALETTE.find(c=>!used.has(c))||PALETTE[state.colors.size%PALETTE.length])}
  return state.colors.get(key);
}
const isTimeReport=()=>!!DIMS[REPORT_DIMS[state.report]?.[0]]?.time;


export {state,colorFor,isTimeReport};
