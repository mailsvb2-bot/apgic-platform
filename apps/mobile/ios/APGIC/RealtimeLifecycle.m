#import <AVFoundation/AVFoundation.h>
#import <Network/Network.h>
#import <React/RCTBridgeModule.h>
#import <React/RCTEventEmitter.h>
#import <UIKit/UIKit.h>

@interface APGICRealtimeLifecycle : RCTEventEmitter <RCTBridgeModule>
@property(nonatomic, assign) BOOL started;
@property(nonatomic, assign) BOOL interrupted;
@property(nonatomic, strong) dispatch_queue_t networkQueue;
@property(nonatomic, strong) nw_path_monitor_t networkMonitor;
@end

@implementation APGICRealtimeLifecycle

RCT_EXPORT_MODULE(APGICRealtimeLifecycle)

- (NSArray<NSString *> *)supportedEvents {
  return @[@"APGICRealtimeLifecycleEvent"];
}

+ (BOOL)requiresMainQueueSetup { return YES; }

RCT_REMAP_METHOD(start,
                 startWithResolver:(RCTPromiseResolveBlock)resolve
                 rejecter:(RCTPromiseRejectBlock)reject) {
  dispatch_async(dispatch_get_main_queue(), ^{
    if (self.started) { resolve(nil); return; }
    self.started = YES;
    NSNotificationCenter *center = NSNotificationCenter.defaultCenter;
    [center addObserver:self selector:@selector(appBackground:) name:UIApplicationDidEnterBackgroundNotification object:nil];
    [center addObserver:self selector:@selector(appForeground:) name:UIApplicationWillEnterForegroundNotification object:nil];
    [center addObserver:self selector:@selector(audioRouteChanged:) name:AVAudioSessionRouteChangeNotification object:nil];
    [center addObserver:self selector:@selector(audioInterrupted:) name:AVAudioSessionInterruptionNotification object:nil];
    [self startNetworkMonitor];
    [self emitAudioRoute];
    resolve(nil);
  });
}

RCT_REMAP_METHOD(stop,
                 stopWithResolver:(RCTPromiseResolveBlock)resolve
                 rejecter:(RCTPromiseRejectBlock)reject) {
  dispatch_async(dispatch_get_main_queue(), ^{
    [self stopNative];
    resolve(nil);
  });
}

RCT_REMAP_METHOD(debugEmit,
                 debugEmit:(NSString *)type
                 route:(NSString * _Nullable)route
                 resolver:(RCTPromiseResolveBlock)resolve
                 rejecter:(RCTPromiseRejectBlock)reject) {
#if DEBUG
  [self emit:type route:route];
  resolve(nil);
#else
  reject(@"REALTIME_DEBUG_DISABLED", @"Debug lifecycle injection is disabled", nil);
#endif
}

- (void)invalidate {
  [self stopNative];
  [super invalidate];
}

- (void)stopNative {
  if (!self.started) return;
  self.started = NO;
  [NSNotificationCenter.defaultCenter removeObserver:self];
  if (self.networkMonitor != nil) {
    nw_path_monitor_cancel(self.networkMonitor);
    self.networkMonitor = nil;
  }
}

- (void)startNetworkMonitor {
  self.networkMonitor = nw_path_monitor_create();
  self.networkQueue = dispatch_queue_create("com.apgic.realtime.network", DISPATCH_QUEUE_SERIAL);
  __weak typeof(self) weakSelf = self;
  nw_path_monitor_set_update_handler(self.networkMonitor, ^(nw_path_t path) {
    __strong typeof(weakSelf) self = weakSelf;
    if (!self || !self.started) return;
    nw_path_status_t status = nw_path_get_status(path);
    if (status != nw_path_status_satisfied) {
      [self emit:@"NETWORK_OFFLINE" route:nil];
    } else if (nw_path_is_expensive(path) || nw_path_is_constrained(path)) {
      [self emit:@"NETWORK_DEGRADED" route:nil];
    } else {
      [self emit:@"NETWORK_ONLINE" route:nil];
    }
  });
  nw_path_monitor_set_queue(self.networkMonitor, self.networkQueue);
  nw_path_monitor_start(self.networkMonitor);
}

- (void)appBackground:(NSNotification *)notification { [self emit:@"APP_BACKGROUND" route:nil]; }
- (void)appForeground:(NSNotification *)notification { [self emit:@"APP_FOREGROUND" route:nil]; }
- (void)audioRouteChanged:(NSNotification *)notification { [self emitAudioRoute]; }

- (void)audioInterrupted:(NSNotification *)notification {
  NSNumber *value = notification.userInfo[AVAudioSessionInterruptionTypeKey];
  if (value.integerValue == AVAudioSessionInterruptionTypeBegan) {
    self.interrupted = YES;
    [self emit:@"INTERRUPTION_BEGAN" route:nil];
  } else if (self.interrupted) {
    self.interrupted = NO;
    [self emit:@"INTERRUPTION_ENDED" route:nil];
  }
}

- (void)emitAudioRoute {
  NSString *route = @"UNKNOWN";
  for (AVAudioSessionPortDescription *output in AVAudioSession.sharedInstance.currentRoute.outputs) {
    NSString *port = output.portType;
    if ([port isEqualToString:AVAudioSessionPortBluetoothA2DP] || [port isEqualToString:AVAudioSessionPortBluetoothHFP] || [port isEqualToString:AVAudioSessionPortBluetoothLE]) route = @"BLUETOOTH";
    else if ([port isEqualToString:AVAudioSessionPortHeadphones] || [port isEqualToString:AVAudioSessionPortUSBAudio]) route = @"WIRED";
    else if ([port isEqualToString:AVAudioSessionPortBuiltInReceiver]) route = @"EARPIECE";
    else if ([port isEqualToString:AVAudioSessionPortBuiltInSpeaker]) route = @"SPEAKER";
    if (![route isEqualToString:@"UNKNOWN"]) break;
  }
  [self emit:@"AUDIO_ROUTE_CHANGED" route:route];
}

- (void)emit:(NSString *)type route:(NSString * _Nullable)route {
  if (!self.started) return;
  NSMutableDictionary *payload = [@{@"type": type} mutableCopy];
  if (route != nil) payload[@"route"] = route;
  dispatch_async(dispatch_get_main_queue(), ^{
    if (self.started) [self sendEventWithName:@"APGICRealtimeLifecycleEvent" body:payload];
  });
}

@end