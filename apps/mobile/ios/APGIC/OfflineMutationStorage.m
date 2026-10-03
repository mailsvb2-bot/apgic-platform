#import <Foundation/Foundation.h>
#import <React/RCTBridgeModule.h>
@import Security;

// APGIC_AUDITED_STORAGE_ADAPTER: INTERNAL_OFFLINE_MUTATION_QUEUE
// APGIC_SECURE_CREDENTIAL_ADAPTER: SYSTEM_KEYSTORE_V1
@interface APGICOfflineMutationStorage : NSObject <RCTBridgeModule>
@end

@implementation APGICOfflineMutationStorage

RCT_EXPORT_MODULE(APGICOfflineMutationStorage)

static NSString *const APGICOfflineMutationQueueKey = @"apgic.offline-mutation.queue.v1";
static NSString *const APGICRemoteConfigKey = @"apgic.remote-config.v1";
static NSString *const APGICCredentialService = @"ru.apgic.mobile.session";
static NSString *const APGICCredentialAccount = @"session-credential-v1";
static const NSUInteger APGICOfflineMutationMaxBytes = 8192;
static const NSUInteger APGICRemoteConfigMaxBytes = 16384;
static const NSUInteger APGICCredentialMaxBytes = 4096;

static NSMutableDictionary *APGICCredentialQuery(void)
{
  return [@{
    (__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
    (__bridge id)kSecAttrService: APGICCredentialService,
    (__bridge id)kSecAttrAccount: APGICCredentialAccount,
  } mutableCopy];
}

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

RCT_REMAP_METHOD(saveCredential,
                 saveCredential:(NSString *)value
                 resolver:(RCTPromiseResolveBlock)resolve
                 rejecter:(RCTPromiseRejectBlock)reject)
{
  NSData *data = [value dataUsingEncoding:NSUTF8StringEncoding];
  if ([data length] > APGICCredentialMaxBytes) {
    reject(@"SECURE_CREDENTIAL_TOO_LARGE", @"Credential exceeds audited size bound", nil);
    return;
  }

  NSMutableDictionary *query = APGICCredentialQuery();
  NSDictionary *update = @{
    (__bridge id)kSecValueData: data,
  };
  OSStatus status = SecItemUpdate(
    (__bridge CFDictionaryRef)query,
    (__bridge CFDictionaryRef)update
  );
  if (status == errSecItemNotFound) {
    query[(__bridge id)kSecValueData] = data;
    query[(__bridge id)kSecAttrAccessible] = (__bridge id)kSecAttrAccessibleWhenUnlockedThisDeviceOnly;
    status = SecItemAdd((__bridge CFDictionaryRef)query, NULL);
  }
  if (status != errSecSuccess) {
    reject(@"SECURE_CREDENTIAL_WRITE_FAILED",
           [NSString stringWithFormat:@"Keychain write failed: %d", (int)status],
           nil);
    return;
  }
  resolve(nil);
}

RCT_REMAP_METHOD(loadCredential,
                 loadCredentialWithResolver:(RCTPromiseResolveBlock)resolve
                 rejecter:(RCTPromiseRejectBlock)reject)
{
  NSMutableDictionary *query = APGICCredentialQuery();
  query[(__bridge id)kSecReturnData] = @YES;
  query[(__bridge id)kSecMatchLimit] = (__bridge id)kSecMatchLimitOne;

  CFTypeRef result = NULL;
  OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &result);
  if (status == errSecItemNotFound) {
    resolve([NSNull null]);
    return;
  }
  if (status != errSecSuccess || result == NULL) {
    reject(@"SECURE_CREDENTIAL_READ_FAILED",
           [NSString stringWithFormat:@"Keychain read failed: %d", (int)status],
           nil);
    if (result != NULL) {
      CFRelease(result);
    }
    return;
  }

  NSData *data = CFBridgingRelease(result);
  NSString *value = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
  if (value == nil) {
    reject(@"SECURE_CREDENTIAL_CORRUPT", @"Keychain credential is not valid UTF-8", nil);
    return;
  }
  resolve(value);
}

static BOOL APGICDeleteCredential(void)
{
  OSStatus status = SecItemDelete((__bridge CFDictionaryRef)APGICCredentialQuery());
  return status == errSecSuccess || status == errSecItemNotFound;
}

RCT_REMAP_METHOD(clearCredential,
                 clearCredentialWithResolver:(RCTPromiseResolveBlock)resolve
                 rejecter:(RCTPromiseRejectBlock)reject)
{
  if (!APGICDeleteCredential()) {
    reject(@"SECURE_CREDENTIAL_CLEAR_FAILED", @"Keychain credential could not be cleared", nil);
    return;
  }
  resolve(nil);
}

RCT_REMAP_METHOD(clearUserScopedState,
                 clearUserScopedStateWithResolver:(RCTPromiseResolveBlock)resolve
                 rejecter:(RCTPromiseRejectBlock)reject)
{
  NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
  [defaults removeObjectForKey:APGICOfflineMutationQueueKey];
  BOOL queueCleared = [defaults synchronize];
  BOOL credentialCleared = APGICDeleteCredential();
  if (!queueCleared || !credentialCleared) {
    reject(@"LOCAL_USER_STATE_CLEAR_FAILED", @"User-scoped local state could not be fully cleared", nil);
    return;
  }
  resolve(nil);
}

+ (BOOL)requiresMainQueueSetup
{
  return NO;
}

@end
