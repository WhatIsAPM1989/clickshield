(() => {
 'use strict';let player=null,total=0,playing=false,width=900,height=600;
 const $=id=>document.getElementById(id);
 const format=ms=>`${Math.floor(ms/60000)}:${String(Math.floor(ms/1000)%60).padStart(2,'0')}`;
 function scale(){const v=$('viewport');const factor=Math.min(v.clientWidth/width,v.clientHeight/height);$('player').style.transform=`scale(${factor})`;}
 function pause(){player?.pause();playing=false;$('play').textContent='Воспроизвести'}
 function update(){if(!player)return;const ms=Math.max(0,Math.min(total,player.getCurrentTime()));$('seek').value=ms;$('time').textContent=`${format(ms)} / ${format(total)}`;if(playing&&ms>=total)pause()}
 $('play').onclick=()=>{if(!player)return;if(playing){pause();return}let at=Number($('seek').value);if(at>=total)at=0;player.play(at);playing=true;$('play').textContent='Пауза'};
 $('seek').oninput=()=>{if(!player)return;const at=Number($('seek').value);if(playing)player.play(at);else player.pause(at);update()};
 $('speed').onchange=()=>player?.setConfig({speed:Number($('speed').value)});
 setInterval(update,200);addEventListener('resize',scale);
 addEventListener('message',e=>{
  if(e.source!==parent||e.data?.type!=='clickshield:replay')return;
  const events=e.data.events;if(!Array.isArray(events)||!events.length)return;
  try{pause();player?.destroy();$('player').replaceChildren();player=null;$('play').disabled=true;
   const first=events.findIndex(e=>e.type===2);if(first<0){$('message').textContent='Нет полного снимка страницы. Запись неполная.';return}
   let start=first;if(first>0&&events[first-1].type===4)start--;
   const meta=events.slice(0,first+1).findLast(e=>e.type===4)?.data; width=Math.max(1,Number(meta?.width)||900);height=Math.max(1,Number(meta?.height)||600);
   const Replayer=window.rrwebReplay.Replayer;
   player=new Replayer(events.slice(start),{root:$('player'),speed:Number($('speed').value),skipInactive:true,UNSAFE_replayCanvas:false,showWarning:false});
   player.on('resize',size=>{width=Math.max(1,size.width);height=Math.max(1,size.height);scale()});
   player.pause(0);total=player.getMetaData().totalTime;$('seek').max=total;$('seek').value=0;$('play').disabled=false;scale();update();
   $('message').textContent=e.data.incomplete?'Запись неполная: некоторые события не доставлены.':'Запись посещения';
  }catch(err){$('message').textContent='Не удалось воспроизвести эту запись.';parent.postMessage({type:'clickshield:replay-error',message:String(err)},'*')}
 });parent.postMessage({type:'clickshield:replay-ready'},'*');
})();
