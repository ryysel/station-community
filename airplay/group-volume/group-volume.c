/* Private LAN group-volume bridge. No receiver/source/ALSA modifications.
 * Resolve OWN live Avahi publication for every operation; never parse logs.
 * Build: arm-linux-gnueabihf-gcc ... -ldbus-1 -lcrypto
 */
#include <arpa/inet.h>
#include <ctype.h>
#include <dbus/dbus.h>
#include <errno.h>
#include <fcntl.h>
#include <openssl/crypto.h>
#include <openssl/hmac.h>
#include <openssl/rand.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/file.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

#define BASE "/data/airplay2"
#define PORT 43792
#define MAX_PEERS 16
static char peers[MAX_PEERS][INET_ADDRSTRLEN];
static size_t peer_count;
static char own[32],name[256];
static unsigned char key[32];
static DBusConnection *bus;
static volatile sig_atomic_t running=1;
static void stopped(int sig) {(void)sig;running=0;}
static int read_line(const char *path,char *out,size_t cap) {
 FILE *f=fopen(path,"r");if(!f)return 0;
 int ok=fgets(out,cap,f)!=NULL;fclose(f);
 if(ok)out[strcspn(out,"\r\n")]=0;return ok;
}
static int known(const char *ip) {
 for(size_t i=0;i<peer_count;i++)if(!strcmp(ip,peers[i]))return 1;return 0;
}
static int add_peer(const char *p) {
 size_t length=strlen(p);struct in_addr address;
 if(length>=INET_ADDRSTRLEN || peer_count==MAX_PEERS ||
    inet_pton(AF_INET,p,&address)!=1 || known(p))return 0;
 unsigned host=ntohl(address.s_addr);
 if((host>>24)==0 || (host>>24)==127 || host>=0xe0000000u)return 0;
 memcpy(peers[peer_count++],p,length+1);return 1;
}
static int read_peers(void) {
 FILE *f=fopen(BASE "/etc/group-volume.peers","r");if(!f)return 0;
 char line[128];peer_count=0;int valid=1;
 while(fgets(line,sizeof(line),f)) {
  char *p=line;while(isspace((unsigned char)*p))p++;
  if(!*p||*p=='#')continue;
  p[strcspn(p," \t\r\n#")]=0;
  if(!add_peer(p)) {valid=0;break;}
 }
 if(ferror(f))valid=0;
 fclose(f);return valid && peer_count>0;
}
static int uuid(const char *s) {
 if(strlen(s)!=36)return 0;
 for(int i=0;i<36;i++)if(i==8||i==13||i==18||i==23) {if(s[i]!='-')return 0;}
 else if(!isxdigit((unsigned char)s[i]))return 0;
 return 1;
}
static int available(void) {
 return access(BASE "/compat/active",F_OK)==0 &&
 access(BASE "/compat/ducked",F_OK)!=0 && access(BASE "/compat/led.suspend",F_OK)!=0;
}
static DBusMessage *call(DBusMessage *m) {
 if(!m)return NULL;
 DBusError e;dbus_error_init(&e);
 DBusMessage *r=dbus_connection_send_with_reply_and_block(bus,m,1200,&e);
 dbus_message_unref(m);dbus_error_free(&e);return r;
}
static int group(char *gid,unsigned *status) {
 gid[0]=0;*status=0;
 DBusMessage *m=dbus_message_new_method_call("org.freedesktop.Avahi","/",
   "org.freedesktop.Avahi.Server","ResolveService");
 dbus_int32_t iface=-1,proto=0;dbus_uint32_t flags=0;
 const char *n=name,*type="_airplay._tcp",*domain="local";
 dbus_message_append_args(m,DBUS_TYPE_INT32,&iface,DBUS_TYPE_INT32,&proto,
   DBUS_TYPE_STRING,&n,DBUS_TYPE_STRING,&type,DBUS_TYPE_STRING,&domain,
   DBUS_TYPE_INT32,&proto,DBUS_TYPE_UINT32,&flags,DBUS_TYPE_INVALID);
 DBusMessage *r=call(m);if(!r)return 0;
 DBusMessageIter it;dbus_message_iter_init(r,&it);
 char address[64]="";unsigned resultflags=0;
 for(int field=0;field<=10;field++) {
  int t=dbus_message_iter_get_arg_type(&it);
  if(field==7 && t==DBUS_TYPE_STRING) {
   const char *s;dbus_message_iter_get_basic(&it,&s);snprintf(address,sizeof(address),"%s",s);
  }
  if(field==9 && t==DBUS_TYPE_ARRAY) {
   DBusMessageIter outer;dbus_message_iter_recurse(&it,&outer);
   while(dbus_message_iter_get_arg_type(&outer)==DBUS_TYPE_ARRAY) {
    DBusMessageIter bytes;dbus_message_iter_recurse(&outer,&bytes);
    unsigned char *data=NULL;int len=0;
    if(dbus_message_iter_get_arg_type(&bytes)==DBUS_TYPE_BYTE) {
     dbus_message_iter_get_fixed_array(&bytes,&data,&len);
     char txt[256];if(len>0 && len<256) {
      memcpy(txt,data,len);txt[len]=0;
      if(!strncmp(txt,"gid=",4) && uuid(txt+4)) {
       for(int i=0;i<=36;i++)gid[i]=(char)toupper((unsigned char)txt[i+4]);
      }
      if(!strncmp(txt,"flags=",6))*status=(unsigned)strtoul(txt+6,NULL,0);
     }
    }
    dbus_message_iter_next(&outer);
   }
  }
  if(field==10 && t==DBUS_TYPE_UINT32)dbus_message_iter_get_basic(&it,&resultflags);
  if(!dbus_message_iter_next(&it))break;
 }
 dbus_message_unref(r);
 /* AVAHI_LOOKUP_RESULT_LOCAL (8), OUR_OWN (4), ReceiverSessionIsActive (17).
  * Idle advertisements contain the device pi as gid: never use those.
  */
 return uuid(gid) && !strcmp(address,own) && (resultflags&12)==12 && (*status&(1u<<17));
}
static int set_volume(int pct) {
 if(!available())return 0;
 /* Guard monitor against the asynchronous remote-control callback. */
 FILE *f=fopen(BASE "/run/group-volume-inhibit.tmp","w");if(!f)return 0;
 fprintf(f,"%ld\n",(long)time(NULL)+4);fclose(f);
 if(rename(BASE "/run/group-volume-inhibit.tmp",BASE "/run/group-volume-inhibit"))return 0;
 DBusMessage *m=dbus_message_new_method_call("org.gnome.ShairportSync",
  "/org/gnome/ShairportSync","org.gnome.ShairportSync.RemoteControl","SetAirplayVolume");
 double db=pct?((double)pct*30.0/100.0-30.0):-144.0;
 dbus_message_append_args(m,DBUS_TYPE_DOUBLE,&db,DBUS_TYPE_INVALID);
 DBusMessage *r=call(m);if(!r)return 0;dbus_message_unref(r);return 1;
}
static void mac(const char *msg,char *hex) {
 unsigned char digest[32];unsigned len=0;
 HMAC(EVP_sha256(),key,sizeof(key),(const unsigned char *)msg,strlen(msg),digest,&len);
 for(unsigned i=0;i<32;i++)sprintf(hex+i*2,"%02x",digest[i]);
}
/* Pure validation separated for adversarial tests, before any D-Bus/audio call. */
static int validate(char *msg,size_t n,const char *ip,long now,
                    char *gid,char *nonce,long *stamp,int *pct) {
 if(!known(ip)||!strcmp(ip,own)||memchr(msg,0,n))return 0;
 char *end=strrchr(msg,' ');if(!end||strlen(end+1)!=64)return 0;
 char digest[65];*end=0;mac(msg,digest);
 if(CRYPTO_memcmp(digest,end+1,64))return 0;
 char sender[32],extra;
 if(sscanf(msg,"AP2V1 %31s %36s %ld %32s %d %c",sender,gid,stamp,nonce,pct,&extra)!=5||
   strcmp(sender,ip)||!uuid(gid)||strlen(nonce)!=32||*pct<0||*pct>100)return 0;
 for(int i=0;i<32;i++)if(!isxdigit((unsigned char)nonce[i]))return 0;
 return *stamp>=now-3 && *stamp<=now+1;
}
static int member(int active,const char *requested,const char *current) {
 return active && uuid(requested) && uuid(current) && !strcmp(requested,current);
}
struct replay_state {char seen[256][33];unsigned pos;long latest[MAX_PEERS];};
static int fresh(struct replay_state *state,int index,const char *nonce,long stamp) {
 if(index<0||(size_t)index>=peer_count||stamp<state->latest[index])return 0;
 for(int i=0;i<256;i++)if(!strcmp(state->seen[i],nonce))return 0;
 snprintf(state->seen[state->pos++%256],33,"%s",nonce);state->latest[index]=stamp;
 return 1;
}
static int config(void) {
 char secret[80];
 if(!read_peers()||!read_line(BASE "/etc/device.ip",own,sizeof(own))||!known(own)||
 !read_line(BASE "/etc/group-volume.name",name,sizeof(name))||!name[0]||
 !read_line(BASE "/etc/group-volume.key",secret,sizeof(secret))||strlen(secret)!=64)return 0;
 for(int i=0;i<32;i++) {
  if(!isxdigit((unsigned char)secret[2*i])||!isxdigit((unsigned char)secret[2*i+1]))return 0;
  char pair[3]={secret[2*i],secret[2*i+1],0};key[i]=(unsigned char)strtoul(pair,NULL,16);
 }
 DBusError e;dbus_error_init(&e);
 bus=dbus_connection_open_private("unix:path=" BASE "/run/bus",&e);
 if(!bus){dbus_error_free(&e);return 0;}
 dbus_connection_set_exit_on_disconnect(bus,FALSE);
 int ok=dbus_bus_register(bus,&e);dbus_error_free(&e);return ok;
}
static int send_volume(int pct) {
 char gid[37];unsigned flags;
 if(!available()||!group(gid,&flags)||!available())return 2;
 unsigned char rnd[16];if(RAND_bytes(rnd,sizeof(rnd))!=1)return 1;
 char nonce[33];for(int i=0;i<16;i++)sprintf(nonce+2*i,"%02x",rnd[i]);
 char msg[256],digest[65];
 snprintf(msg,sizeof(msg),"AP2V1 %s %s %ld %s %d",own,gid,(long)time(NULL),nonce,pct);
 mac(msg,digest);strcat(msg," ");strcat(msg,digest);
 int s=socket(AF_INET,SOCK_DGRAM,0);if(s<0)return 1;
 struct sockaddr_in local={.sin_family=AF_INET};inet_pton(AF_INET,own,&local.sin_addr);
 if(bind(s,(struct sockaddr *)&local,sizeof(local))){close(s);return 1;}
 /* Send once: duplicate/reordered commands must never overwrite newer phone changes. */
 int ok=1;
 for(size_t i=0;i<peer_count;i++)if(strcmp(peers[i],own)) {
  struct sockaddr_in to={.sin_family=AF_INET,.sin_port=htons(PORT)};
  inet_pton(AF_INET,peers[i],&to.sin_addr);
  if(sendto(s,msg,strlen(msg),0,(struct sockaddr *)&to,sizeof(to))!=(ssize_t)strlen(msg))ok=0;
 }
 close(s);printf("group-volume sent pct=%d\n",pct);return ok?0:1;
}
static int listen_peer(int dry) {
 int lock=open(BASE "/run/group-volume.lock",O_CREAT|O_RDWR,0600);
 if(lock<0||flock(lock,LOCK_EX|LOCK_NB))return 1;
 int s=socket(AF_INET,SOCK_DGRAM,0);if(s<0)return 1;
 struct sockaddr_in local={.sin_family=AF_INET,.sin_port=htons(PORT)};
 inet_pton(AF_INET,own,&local.sin_addr);
 if(bind(s,(struct sockaddr *)&local,sizeof(local)))return 1;
 struct timeval timeout={.tv_sec=1};setsockopt(s,SOL_SOCKET,SO_RCVTIMEO,&timeout,sizeof(timeout));
 signal(SIGTERM,stopped);signal(SIGINT,stopped);
 /* Cache valid nonces before membership checks too. Never replay after a group change. */
 struct replay_state state={0};
 while(running) {
  char msg[512];struct sockaddr_in from;socklen_t flen=sizeof(from);
  ssize_t n=recvfrom(s,msg,sizeof(msg)-1,0,(struct sockaddr *)&from,&flen);
  if(n<=0)continue;msg[n]=0;
  char ip[32];inet_ntop(AF_INET,&from.sin_addr,ip,sizeof(ip));
  char gid[37],nonce[33];long stamp;int pct;
  if(!validate(msg,(size_t)n,ip,(long)time(NULL),gid,nonce,&stamp,&pct))continue;
  int index=0;while((size_t)index<peer_count&&strcmp(ip,peers[index]))index++;
  if(!fresh(&state,index,nonce,stamp))continue;
  char current[37];unsigned flags;
  if(!available()||!group(current,&flags)||!member(available(),gid,current)) {
   puts("group-volume ignored: inactive, busy or different group");fflush(stdout);continue;
  }
  /* No shell, no ALSA changes, no forwarding of received commands. */
  int ok=dry?1:set_volume(pct);
  printf("group-volume %s pct=%d peer=%s\n",dry?"dry-run":(ok?"applied":"failed"),pct,ip);fflush(stdout);
 }
 close(s);close(lock);return 0;
}
int main(int argc,char **argv) {
 umask(077);if(argc<2||!config())return 1;
 int rc=1;
 if(!strcmp(argv[1],"group")) {
  char gid[37];unsigned flags;int ok=group(gid,&flags);
  printf("session=%d active=%d flags=0x%x gid=%s\n",ok,available(),flags,ok?gid:"none");rc=ok?0:2;
 } else if(!strcmp(argv[1],"send")&&argc==3) {
  char *end;long pct=strtol(argv[2],&end,10);
  if(*argv[2]&&!*end&&pct>=0&&pct<=100)rc=send_volume((int)pct);
 } else if(!strcmp(argv[1],"listen"))rc=listen_peer(argc==3&&!strcmp(argv[2],"--dry-run"));
 dbus_connection_close(bus);dbus_connection_unref(bus);return rc;
}
