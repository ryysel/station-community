/* Include implementation to test guards without D-Bus, sockets or audio. */
#define main bridge_main
#include "group-volume.c"
#undef main
#include <assert.h>
static const char *g="11111111-2222-3333-4444-555555555555";
static int check(const char *body,const char *ip,long now,int damage) {
 char msg[512],digest[65],gid[37],nonce[33];long stamp;int pct;
 mac(body,digest);snprintf(msg,sizeof(msg),"%s %s",body,digest);
 if(damage)msg[strlen(msg)-1]^=1;
 return validate(msg,strlen(msg),ip,now,gid,nonce,&stamp,&pct);
}
int main(void) {
 assert(!add_peer("127.0.0.1"));assert(!add_peer("224.0.0.1"));
 assert(!add_peer("not-an-address"));assert(add_peer("192.0.2.10"));
 assert(!add_peer("192.0.2.10"));
 for(size_t i=1;i<MAX_PEERS;i++) {
  char ip[INET_ADDRSTRLEN];snprintf(ip,sizeof(ip),"198.51.100.%u",(unsigned)i);
  assert(add_peer(ip));
 }
 assert(!add_peer("203.0.113.1"));
 peer_count=0;
 assert(add_peer("192.0.2.10"));assert(add_peer("192.0.2.20"));
 assert(add_peer("192.0.2.30"));assert(add_peer("192.0.2.40"));
 strcpy(own,"192.0.2.10");memset(key,0,sizeof(key));
 assert(uuid(g));assert(!uuid(""));assert(!uuid("11111111-2222-3333-4444-55555555555Z"));
 assert(member(1,g,g));assert(!member(0,g,g));assert(!member(1,g,""));
 assert(!member(1,g,"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"));
 struct replay_state state={0};
 assert(fresh(&state,1,"0123456789abcdef0123456789abcdef",1000));
 assert(!fresh(&state,1,"0123456789abcdef0123456789abcdef",1000));
 assert(!fresh(&state,1,"1123456789abcdef0123456789abcdef",999));
 assert(fresh(&state,1,"1123456789abcdef0123456789abcdef",1001));
 assert(!fresh(&state,4,"2123456789abcdef0123456789abcdef",1002));
 const char *body="AP2V1 192.0.2.20 11111111-2222-3333-4444-555555555555 1000 0123456789abcdef0123456789abcdef 73";
 assert(check(body,"192.0.2.20",1000,0));
 assert(!check(body,"192.0.2.20",1000,1)); /* invalid HMAC */
 assert(!check(body,"192.0.2.99",1000,0)); /* unlisted peer */
 assert(!check(body,"192.0.2.30",1000,0)); /* source spoof */
 assert(!check(body,"192.0.2.20",1004,0)); /* expired */
 assert(!check(body,"192.0.2.20",998,0)); /* future */
 assert(!check("AP2V1 192.0.2.10 11111111-2222-3333-4444-555555555555 1000 0123456789abcdef0123456789abcdef 73","192.0.2.10",1000,0));
 assert(!check("AP2V1 192.0.2.20 11111111-2222-3333-4444-555555555555 1000 0123456789abcdef0123456789abcdef 101","192.0.2.20",1000,0));
 assert(!check("AP2V1 192.0.2.20 11111111-2222-3333-4444-555555555555 1000 0123456789abcdef0123456789abcdef -1","192.0.2.20",1000,0));
 assert(check("AP2V1 192.0.2.20 11111111-2222-3333-4444-555555555555 1000 0123456789abcdef0123456789abcdef 0","192.0.2.20",1000,0));
 assert(check("AP2V1 192.0.2.20 11111111-2222-3333-4444-555555555555 1000 0123456789abcdef0123456789abcdef 100","192.0.2.20",1000,0));
 assert(!check("AP2V1 192.0.2.20 11111111-2222-3333-4444-555555555555 1000 0123456789abcdef0123456789abcdef 1 trailing","192.0.2.20",1000,0));
 assert(!check("AP2V1 192.0.2.20 not-a-uuid 1000 0123456789abcdef0123456789abcdef 73","192.0.2.20",1000,0));
 assert(!check("AP2V1 192.0.2.20 11111111-2222-3333-4444-555555555555 1000 Z123456789abcdef0123456789abcdef 73","192.0.2.20",1000,0));
 puts("PASS: UUID, active/exact membership, HMAC, allowlist, expiry, replay, ordering, bounds, malformed payloads");return 0;
}
