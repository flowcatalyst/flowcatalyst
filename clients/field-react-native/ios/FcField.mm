// The `FcField` TurboModule (spec: src/NativeFcField.ts). Every method forwards to the Swift class `FcFieldHost`,
// which owns the field-ios `FieldSession`, the device key and the Keychain store. This file holds no logic.
//
// Events (SessionChanged, SessionEnded, ...) go out through RCTEventEmitter, which is what `new NativeEventEmitter(
// native)` in FieldClient subscribes to. Rejections carry the code and, as the NSError user-info, `status`,
// `retryAfterSeconds` and `violations` when present.
#import <React/RCTBridgeModule.h>
#import <React/RCTEventEmitter.h>
#import <ReactCommon/RCTTurboModule.h>
#import <FcFieldSpec/FcFieldSpec.h>

#import "FlowcatalystFieldReactNative-Swift.h"

@interface FcField : RCTEventEmitter <NativeFcFieldSpec>
@end

@implementation FcField {
  FcFieldHost *_host;
}

RCT_EXPORT_MODULE(FcField)

+ (BOOL)requiresMainQueueSetup
{
  return NO;
}

- (instancetype)init
{
  if ((self = [super init])) {
    _host = [FcFieldHost new];
    __weak FcField *weakSelf = self;
    _host.eventSink = ^(NSString *name, NSDictionary<NSString *, id> *body) {
      [weakSelf sendEventWithName:name body:body];
    };
  }
  return self;
}

- (void)invalidate
{
  [_host invalidate];
  _host.eventSink = nil;
  [super invalidate];
}

- (NSArray<NSString *> *)supportedEvents
{
  return @[ @"SessionChanged", @"SessionEnded", @"PinChangeRequired", @"ExpiryWarning", @"Locked", @"WorkerLocked", @"UnlockKeyChanged" ];
}

- (std::shared_ptr<facebook::react::TurboModule>)getTurboModule:(const facebook::react::ObjCTurboModule::InitParams &)params
{
  return std::make_shared<facebook::react::NativeFcFieldSpecJSI>(params);
}

// MARK: promise plumbing

static void (^FcRejecter(RCTPromiseRejectBlock reject))(NSString *, NSString *, NSDictionary<NSString *, id> *)
{
  return ^(NSString *code, NSString *message, NSDictionary<NSString *, id> *info) {
    NSError *err = info.count > 0 ? [NSError errorWithDomain:@"io.flowcatalyst.field" code:0 userInfo:info] : nil;
    reject(code, message, err);
  };
}

// MARK: session owner

- (void)configure:(NSDictionary *)config resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host configure:config resolve:resolve reject:FcRejecter(reject)];
}

- (void)restore:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host restore:resolve reject:FcRejecter(reject)];
}

- (void)getState:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host getState:resolve reject:FcRejecter(reject)];
}

- (void)enrol:(NSString *)qrOrCode info:(NSDictionary *)info resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host enrol:qrOrCode info:info resolve:resolve reject:FcRejecter(reject)];
}

- (void)login:(NSDictionary *)creds resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host login:creds resolve:resolve reject:FcRejecter(reject)];
}

- (void)redeemSignInCode:(NSString *)code resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host redeemSignInCode:code resolve:resolve reject:FcRejecter(reject)];
}

- (void)actAs:(NSDictionary *)target resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host actAs:target resolve:resolve reject:FcRejecter(reject)];
}

- (void)endImpersonation:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host endImpersonation:resolve reject:FcRejecter(reject)];
}

- (void)changePin:(NSString *)currentPin newPin:(NSString *)newPin resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host changePin:currentPin newPin:newPin resolve:resolve reject:FcRejecter(reject)];
}

- (void)getPhoto:(NSString *)size haveVersion:(NSString *)haveVersion resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host getPhoto:size haveVersion:haveVersion resolve:resolve reject:FcRejecter(reject)];
}

- (void)setPhoto:(NSString *)bytesB64u contentType:(NSString *)contentType resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host setPhoto:bytesB64u contentType:contentType resolve:resolve reject:FcRejecter(reject)];
}

- (void)removePhoto:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host removePhoto:resolve reject:FcRejecter(reject)];
}

- (void)logout:(NSString *)scope resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host logout:scope resolve:resolve reject:FcRejecter(reject)];
}

- (void)unlock:(NSString *)prompt resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host unlock:prompt resolve:resolve reject:FcRejecter(reject)];
}

// MARK: unlock keys

- (void)unlockKeyAvailability:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host unlockKeyAvailability:resolve reject:FcRejecter(reject)];
}

- (void)unlockKey:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host unlockKey:resolve reject:FcRejecter(reject)];
}

- (void)enableUnlockKey:(NSDictionary *)prompt resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host enableUnlockKey:prompt resolve:resolve reject:FcRejecter(reject)];
}

- (void)loginWithUnlockKey:(NSDictionary *)prompt resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host loginWithUnlockKey:prompt resolve:resolve reject:FcRejecter(reject)];
}

- (void)disableUnlockKey:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host disableUnlockKey:resolve reject:FcRejecter(reject)];
}

- (void)refresh:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host refresh:resolve reject:FcRejecter(reject)];
}

- (void)headersFor:(NSString *)method url:(NSString *)url resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host headersFor:method url:url resolve:resolve reject:FcRejecter(reject)];
}

- (void)headersForRetry:(NSString *)method
                    url:(NSString *)url
                  nonce:(NSString *)nonce
                resolve:(RCTPromiseResolveBlock)resolve
                 reject:(RCTPromiseRejectBlock)reject
{
  [_host headersForRetry:method url:url nonce:nonce resolve:resolve reject:FcRejecter(reject)];
}

- (void)noteNonce:(NSString *)url nonce:(NSString *)nonce
{
  [_host noteNonce:url nonce:nonce];
}

- (void)keepAlive:(BOOL)on
{
  [_host keepAlive:on];
}

// addListener: / removeListeners: come from RCTEventEmitter.

// MARK: key / store

- (void)getOrCreateDeviceKey:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host getOrCreateDeviceKey:resolve reject:FcRejecter(reject)];
}

- (void)sign:(NSString *)dataB64u resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host sign:dataB64u resolve:resolve reject:FcRejecter(reject)];
}

- (void)deleteDeviceKey:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host deleteDeviceKey:resolve reject:FcRejecter(reject)];
}

- (void)storeGet:(NSString *)key prompt:(NSString *)prompt resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host storeGet:key prompt:prompt resolve:resolve reject:FcRejecter(reject)];
}

- (void)storeSet:(NSString *)key
            value:(NSString *)value
 requireBiometric:(BOOL)requireBiometric
          resolve:(RCTPromiseResolveBlock)resolve
           reject:(RCTPromiseRejectBlock)reject
{
  [_host storeSet:key value:value requireBiometric:requireBiometric resolve:resolve reject:FcRejecter(reject)];
}

- (void)storeDelete:(NSString *)key resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host storeDelete:key resolve:resolve reject:FcRejecter(reject)];
}

- (void)biometricAvailable:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
{
  [_host biometricAvailable:resolve reject:FcRejecter(reject)];
}

@end
