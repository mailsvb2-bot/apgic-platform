#import <Foundation/Foundation.h>
#import <React/RCTBridgeModule.h>

// APGIC_AUDITED_STORAGE_ADAPTER: INTERNAL_OFFLINE_MUTATION_QUEUE
@interface APGICOfflineMutationStorage : NSObject <RCTBridgeModule>
@end

@implementation APGICOfflineMutationStorage

RCT_EXPORT_MODULE(APGICOfflineMutationStorage)

static NSString *const APGICOfflineMutationQueueKey = @"apgic.offline-mutation.queue.v1";
static NSString *const APGICRemoteConfigKey = @"apgic.remote-config.v1";
static const NSUInteger APGICOfflineMutationMaxBytes = 8192;
static const NSUInteger APGICRemoteConfigMaxBytes = 16384;

RCT_REMAP_METHOD(load,
                 loadWithResolver:(RCTPromiseResolveBlock)resolve
                 rejecter:(RCTPromiseRejectBlock)reject)
{
  NSString *value = [[NSUserDefaults standardUserDefaults] stringForKey:APGICOfflineMutationQueueKey];
  resolve(value ?: [NSNull null]);
}

RCT_REMAP_METHOD(save,
                 save:(NSString *)value
                 resolver:(RCTPromiseResolveBlock)resolve
                 rejecter:(RCTPromiseRejectBlock)reject)
{
  if ([value lengthOfBytesUsingEncoding:NSUTF8StringEncoding] > APGICOfflineMutationMaxBytes) {
    reject(@"OFFLINE_MUTATION_STORAGE_TOO_LARGE",
           @"Offline mutation queue exceeds audited size bound",
           nil);
    return;
  }
  NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
  [defaults setObject:value forKey:APGICOfflineMutationQueueKey];
  if (![defaults synchronize]) {
    reject(@"OFFLINE_MUTATION_STORAGE_WRITE_FAILED",
           @"Offline mutation queue could not be persisted",
           nil);
    return;
  }
  resolve(nil);
}

RCT_REMAP_METHOD(clear,
                 clearWithResolver:(RCTPromiseResolveBlock)resolve
                 rejecter:(RCTPromiseRejectBlock)reject)
{
  NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
  [defaults removeObjectForKey:APGICOfflineMutationQueueKey];
  if (![defaults synchronize]) {
    reject(@"OFFLINE_MUTATION_STORAGE_CLEAR_FAILED",
           @"Offline mutation queue could not be cleared",
           nil);
    return;
  }
  resolve(nil);
}

RCT_REMAP_METHOD(loadRemoteConfig,
                 loadRemoteConfigWithResolver:(RCTPromiseResolveBlock)resolve
                 rejecter:(RCTPromiseRejectBlock)reject)
{
  NSString *value = [[NSUserDefaults standardUserDefaults] stringForKey:APGICRemoteConfigKey];
  resolve(value ?: [NSNull null]);
}

RCT_REMAP_METHOD(saveRemoteConfig,
                 saveRemoteConfig:(NSString *)value
                 resolver:(RCTPromiseResolveBlock)resolve
                 rejecter:(RCTPromiseRejectBlock)reject)
{
  if ([value lengthOfBytesUsingEncoding:NSUTF8StringEncoding] > APGICRemoteConfigMaxBytes) {
    reject(@"REMOTE_CONFIG_STORAGE_TOO_LARGE", @"Remote config exceeds audited size bound", nil);
    return;
  }
  NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
  [defaults setObject:value forKey:APGICRemoteConfigKey];
  if (![defaults synchronize]) {
    reject(@"REMOTE_CONFIG_STORAGE_WRITE_FAILED", @"Remote config could not be persisted", nil);
    return;
  }
  resolve(nil);
}

RCT_REMAP_METHOD(clearRemoteConfig,
                 clearRemoteConfigWithResolver:(RCTPromiseResolveBlock)resolve
                 rejecter:(RCTPromiseRejectBlock)reject)
{
  NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
  [defaults removeObjectForKey:APGICRemoteConfigKey];
  if (![defaults synchronize]) {
    reject(@"REMOTE_CONFIG_STORAGE_CLEAR_FAILED", @"Remote config could not be cleared", nil);
    return;
  }
  resolve(nil);
}

+ (BOOL)requiresMainQueueSetup
{
  return NO;
}

@end
