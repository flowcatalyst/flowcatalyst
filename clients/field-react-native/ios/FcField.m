#import <React/RCTBridgeModule.h>

@interface RCT_EXTERN_MODULE (FcField, NSObject)

RCT_EXTERN_METHOD(getOrCreateDeviceKey : (RCTPromiseResolveBlock)resolve reject : (RCTPromiseRejectBlock)reject)
RCT_EXTERN_METHOD(sign : (NSString *)dataB64u resolve : (RCTPromiseResolveBlock)resolve reject : (RCTPromiseRejectBlock)reject)
RCT_EXTERN_METHOD(deleteDeviceKey : (RCTPromiseResolveBlock)resolve reject : (RCTPromiseRejectBlock)reject)
RCT_EXTERN_METHOD(storeGet : (NSString *)key prompt : (NSString *)prompt resolve : (RCTPromiseResolveBlock)resolve reject : (RCTPromiseRejectBlock)reject)
RCT_EXTERN_METHOD(storeSet : (NSString *)key value : (NSString *)value requireBiometric : (BOOL)requireBiometric resolve : (RCTPromiseResolveBlock)resolve reject : (RCTPromiseRejectBlock)reject)
RCT_EXTERN_METHOD(storeDelete : (NSString *)key resolve : (RCTPromiseResolveBlock)resolve reject : (RCTPromiseRejectBlock)reject)
RCT_EXTERN_METHOD(biometricAvailable : (RCTPromiseResolveBlock)resolve reject : (RCTPromiseRejectBlock)reject)

+ (BOOL)requiresMainQueueSetup
{
  return NO;
}

@end
