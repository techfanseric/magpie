//go:build darwin && cgo

package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
#include <string.h>
#include <math.h>
#import <Cocoa/Cocoa.h>

typedef struct {
	const void *icon;
	int iconLen;
	int mono;
	int plain; // no logo: the rows alone, a thin line before
	int ring, known, failed, low, paced, active;
	double share, pace;
	const char *id;
	const char *title;
	const char *letter;
	const char *rows; // one or two, a newline between
} mpTrayCell;

// Metrics, in points: the menu bar is 22 high (24 beside a notch), and a
// cell is a 14pt logo with its two windows beside it, stacked as small
// digits, each window's figures right-aligned over the other's. A cell
// without its logo is the digits alone, a hairline mpSepGap either side
// between it and the one before.
static const CGFloat mpLogo = 14, mpLogoGap = 3, mpCellGap = 8, mpBirdGap = 2, mpRowGap = 3.5,
	mpPlainGap = 3, mpSepGap = 5, mpSep = 1, mpSepHigh = 13;

// mpRowFont is the digits' font, in the weight the menu bar's other
// figures use (Surge's speeds, iStat Menus'): two stacked rows in the small
// system font, a lone row in the menu bar's own font (13pt), both regular
// and with digits of one width, so a figure that changes doesn't shift its
// neighbours. Semibold 9.5pt stacked and medium 11pt alone read heavier
// than everything beside them (qingnianxiaozhe on X).
static NSFont *mpRowFont(int rows) {
	if (rows > 1) return [NSFont monospacedDigitSystemFontOfSize:10 weight:NSFontWeightRegular];
	return [NSFont monospacedDigitSystemFontOfSize:[NSFont menuBarFontOfSize:0].pointSize weight:NSFontWeightRegular];
}

// mpRowFontInfo says what mpRowFont gives for rows: its size, its weight
// trait, and whether "1" is as wide as "8".
static void mpRowFontInfo(int rows, CGFloat *size, CGFloat *weight, int *mono, CGFloat *menuBar) {
	@autoreleasepool {
		NSFont *f = mpRowFont(rows);
		*size = f.pointSize;
		*weight = [[[f.fontDescriptor objectForKey:NSFontTraitsAttribute] objectForKey:NSFontWeightTrait] doubleValue];
		NSDictionary *a = @{NSFontAttributeName: f};
		*mono = [@"1111" sizeWithAttributes:a].width == [@"8888" sizeWithAttributes:a].width;
		*menuBar = [NSFont menuBarFontOfSize:0].pointSize;
	}
}

// mpCells reads the cells Go sends into dictionaries the drawing keeps.
static NSArray *mpCells(mpTrayCell *cells, int n) {
	NSMutableArray *out = [NSMutableArray arrayWithCapacity:n];
	for (int i = 0; i < n; i++) {
		NSMutableDictionary *c = [NSMutableDictionary dictionary];
		if (cells[i].icon != NULL && cells[i].iconLen > 0) {
			NSImage *im = [[NSImage alloc] initWithData:[NSData dataWithBytes:cells[i].icon length:cells[i].iconLen]];
			// an SVG an older system can't read comes back as nothing, or empty
			if (im != nil && im.representations.count > 0) c[@"icon"] = im;
			[im release];
		}
		c[@"active"] = @(cells[i].active);
		c[@"ring"] = @(cells[i].ring); c[@"known"] = @(cells[i].known);
		c[@"failed"] = @(cells[i].failed); c[@"low"] = @(cells[i].low);
		c[@"paced"] = @(cells[i].paced); c[@"share"] = @(cells[i].share); c[@"pace"] = @(cells[i].pace);
		c[@"id"] = [NSString stringWithUTF8String:cells[i].id ? cells[i].id : ""];
		c[@"title"] = [NSString stringWithUTF8String:cells[i].title ? cells[i].title : ""];
		c[@"mono"] = @(cells[i].mono != 0);
		c[@"plain"] = @(cells[i].plain != 0);
		c[@"letter"] = [NSString stringWithUTF8String:cells[i].letter ? cells[i].letter : ""];
		NSArray *rows = [[NSString stringWithUTF8String:cells[i].rows ? cells[i].rows : ""] componentsSeparatedByString:@"\n"];
		if (rows.count > 2) rows = [rows subarrayWithRange:NSMakeRange(0, 2)];
		c[@"rows"] = rows;
		[out addObject:c];
	}
	return out;
}

static CGFloat mpColumn(NSDictionary *c) {
	if ([c[@"ring"] boolValue]) return 22;
	NSArray *rows = c[@"rows"];
	NSDictionary *attrs = @{NSFontAttributeName: mpRowFont((int)rows.count)};
	CGFloat w = 0;
	for (NSString *r in rows) w = MAX(w, ceil([r sizeWithAttributes:attrs].width));
	return w;
}

// mpLead is the room before a cell's digits: its logo, and the gap from the
// cell before (the bird's for the first); a cell without its logo has the
// line between it and the one before instead.
static CGFloat mpLead(NSDictionary *c, BOOL first) {
	if ([c[@"ring"] boolValue]) return first ? 0 : 2;
	if ([c[@"plain"] boolValue]) return first ? mpPlainGap : mpSepGap + mpSep + mpSepGap;
	return (first ? 0 : mpCellGap) + mpLogo + mpLogoGap;
}

// mpIn is the room before the first cell: the bird's and the gap after
// it, or none when the bird is left out (settings.TrayNoBird).
static CGFloat mpIn(NSImage *bird, CGFloat h) { return bird != nil ? h + mpBirdGap : 0; }

static CGFloat mpWidth(NSArray *cells, CGFloat h, CGFloat in) {
	CGFloat w = in;
	BOOL first = YES;
	for (NSDictionary *c in cells) {
		w += mpLead(c, first) + mpColumn(c);
		first = NO;
	}
	return ceil(w + 1);
}

// mpTinted draws a black glyph in the text colour of what it is drawn on
// (the menu bar's, light or dark), as a template image is: in a layer of
// its own, so filling it touches nothing drawn before.
static void mpTinted(NSImage *im, NSRect r) {
	CGContextRef cg = [[NSGraphicsContext currentContext] CGContext];
	CGContextBeginTransparencyLayerWithRect(cg, NSRectToCGRect(r), NULL);
	[im drawInRect:r fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1 respectFlipped:YES hints:nil];
	[[NSColor labelColor] set];
	NSRectFillUsingOperation(r, NSCompositingOperationSourceIn);
	CGContextEndTransparencyLayer(cg);
}

// mpMasked draws a logo as a template image would be: one ink, the bar's
// text colour. Its shape is what is opaque, less what is near white when
// the rest isn't (Codex's white glyph on a blue tile is the tile with the
// glyph cut out); a logo that is all one light colour keeps its whole
// shape. Logos come with any margin round them in their files, so the
// shape is cut to its own bounds and fitted to r, centred: each as large
// as the next, a solid tile (fuller to the eye) a little smaller.
static void mpMasked(NSImage *im, NSRect r) {
	const int n = 128; // 14pt at more than any backing scale, margin and all
	size_t stride = n * 4;
	unsigned char *px = calloc(n * stride, 1);
	CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
	CGContextRef bm = CGBitmapContextCreate(px, n, n, 8, stride, cs, (CGBitmapInfo)kCGImageAlphaPremultipliedLast);
	CGColorSpaceRelease(cs);
	if (bm == NULL) {
		free(px);
		return;
	}
	// its own shape, not stretched to a square
	NSSize is = im.size;
	CGFloat k = (is.width > 0 && is.height > 0) ? n / MAX(is.width, is.height) : 0;
	NSRect at = k > 0 ? NSMakeRect((n - is.width * k) / 2, (n - is.height * k) / 2, is.width * k, is.height * k) : NSMakeRect(0, 0, n, n);
	[NSGraphicsContext saveGraphicsState];
	[NSGraphicsContext setCurrentContext:[NSGraphicsContext graphicsContextWithCGContext:bm flipped:NO]];
	[im drawInRect:at fromRect:NSZeroRect operation:NSCompositingOperationCopy fraction:1];
	[NSGraphicsContext restoreGraphicsState];
	// how much of it is light: the least of its channels, unpremultiplied
	int solid = 0, light = 0;
	for (int i = 0; i < n * n; i++) {
		unsigned char *p = px + i * 4;
		if (p[3] < 128) continue;
		solid++;
		if (MIN(MIN(p[0], p[1]), p[2]) * 255 / p[3] > 200) light++;
	}
	BOOL cut = solid > 0 && light < solid * 0.85;
	int x0 = n, y0 = n, x1 = -1, y1 = -1;
	for (int i = 0; i < n * n; i++) {
		unsigned char *p = px + i * 4;
		CGFloat a = p[3] / 255.0;
		if (cut && p[3] > 0) {
			// none from 200 up, whole from 150 down: a soft edge
			CGFloat w = MIN(MIN(p[0], p[1]), p[2]) * 255.0 / p[3];
			a *= MAX(0, MIN(1, (200 - w) / 50));
		}
		p[0] = p[1] = p[2] = 0;
		p[3] = (unsigned char)round(a * 255);
		if (p[3] > 64) {
			int x = i % n, y = i / n;
			x0 = MIN(x0, x), x1 = MAX(x1, x), y0 = MIN(y0, y), y1 = MAX(y1, y);
		}
	}
	CGImageRef whole = CGBitmapContextCreateImage(bm);
	CGContextRelease(bm);
	free(px);
	if (whole == NULL) return;
	if (x1 < x0) {
		CGImageRelease(whole);
		return;
	}
	// what the shape fills of its bounds, before the cut
	CGRect box = CGRectMake(x0, y0, x1 - x0 + 1, y1 - y0 + 1);
	CGImageRef mask = CGImageCreateWithImageInRect(whole, box);
	CGImageRelease(whole);
	if (mask == NULL) return;
	CGFloat side = r.size.width * (solid > box.size.width * box.size.height * 0.8 ? 0.9 : 1);
	CGFloat f = side / MAX(box.size.width, box.size.height);
	NSSize sz = NSMakeSize(box.size.width * f, box.size.height * f);
	NSRect fit = NSMakeRect(NSMidX(r) - sz.width / 2, NSMidY(r) - sz.height / 2, sz.width, sz.height);
	NSImage *glyph = [[NSImage alloc] initWithCGImage:mask size:sz];
	CGImageRelease(mask);
	mpTinted(glyph, fit);
	[glyph release];
}

// mpRows draws a cell's digits in its column from x: the digits (cap
// height) centred as a block, each row right-aligned; drawAtPoint takes the
// line's top, its baseline an ascender below.
static void mpRows(NSDictionary *c, CGFloat x, CGFloat h, NSColor *ink) {
	NSArray *rows = c[@"rows"];
	NSFont *f = mpRowFont((int)rows.count);
	NSDictionary *a = @{NSFontAttributeName: f, NSForegroundColorAttributeName: ink};
	CGFloat col = mpColumn(c);
	CGFloat cap = f.capHeight;
	CGFloat top = (h - (cap * rows.count + mpRowGap * (rows.count - 1))) / 2;
	for (NSUInteger i = 0; i < rows.count; i++) {
		NSString *r = rows[i];
		CGFloat base = top + cap * (i + 1) + mpRowGap * i;
		CGFloat w = [r sizeWithAttributes:a].width;
		[r drawAtPoint:NSMakePoint(x + col - w, base - f.ascender) withAttributes:a];
	}
}

// Live activity is updated independently of the slowly refreshed quotas.
// All access is on AppKit's main thread. Preview PNGs use a fixed phase.
static NSDictionary *mpActivities;
static NSTimer *mpMotion;
static BOOL mpReduced;
static double mpPreviewPhase = -1;
static BOOL mpPreviewReduced;
static NSString *mpBaseTip;

static int mpActiveCount(NSDictionary *c) {
	NSDictionary *a = mpActivities[[c[@"id"] lowercaseString]];
	return a != nil ? [a[@"count"] intValue] : [c[@"active"] intValue];
}

static void mpRingWaves(NSDictionary *c, NSPoint center, CGFloat k, NSColor *label) {
	int n = MIN(5, MAX(0, mpActiveCount(c)));
	if (!n) return;
	BOOL reduced = mpPreviewPhase >= 0 ? mpPreviewReduced : mpReduced || NSWorkspace.sharedWorkspace.accessibilityDisplayShouldReduceMotion;
	double phase = reduced ? 0 : mpPreviewPhase >= 0 ? mpPreviewPhase : fmod(NSProcessInfo.processInfo.systemUptime / 1.8, 1);
	NSColor *ink = NSColor.controlAccentColor;
	for (int i = 0; i < n; i++) {
		double head = fmod(1 - phase + (double)i/n, 1);
		for (int j = 0; j < 16; j++) {
			double lo = head + 0.16*j/16, hi = head + 0.16*(j+1)/16;
			for (int wrap = 0; wrap <= 1; wrap++) {
				double a = MAX(lo-wrap, 52.0/360), b = MIN(hi-wrap, 308.0/360);
				if (b <= a) continue; // the top opening stays empty as a wave crosses it
				NSBezierPath *p = [NSBezierPath bezierPath];
				[p appendBezierPathWithArcWithCenter:center radius:155.975*k startAngle:-90+a*360 endAngle:-90+b*360 clockwise:NO];
				p.lineWidth = 1.15; p.lineCapStyle = NSLineCapStyleRound;
				[[ink colorWithAlphaComponent:pow(1-(double)j/16,0.65)] setStroke]; [p stroke];
			}
		}
	}
}

// mpRing draws a 22pt quota unit: the provider initial above an open arc,
// with an optional static reserve/deficit fan. Exact figures stay in the tooltip.
static void mpRingArc(NSPoint center, CGFloat radius, CGFloat width, CGFloat share, NSColor *ink) {
	if (share <= 0) return;
	NSBezierPath *p = [NSBezierPath bezierPath];
	[p appendBezierPathWithArcWithCenter:center radius:radius startAngle:-38 endAngle:-38 + 256 * MIN(1, share) clockwise:NO];
	p.lineWidth = width; p.lineCapStyle = NSLineCapStyleRound;
	[ink setStroke]; [p stroke];
}

static void mpRingFan(NSPoint c, CGFloat inner, CGFloat outer, CGFloat middle, CGFloat fraction, NSColor *ink) {
	if (fraction <= 0) return;
	CGFloat half = 48 * MIN(1, fraction);
	NSBezierPath *p = [NSBezierPath bezierPath];
	[p appendBezierPathWithArcWithCenter:c radius:outer startAngle:middle-half endAngle:middle+half clockwise:NO];
	[p appendBezierPathWithArcWithCenter:c radius:inner startAngle:middle+half endAngle:middle-half clockwise:YES];
	[p closePath]; [ink setFill]; [p fill];
}

static void mpRing(NSDictionary *c, CGFloat x, CGFloat h, NSColor *label) {
	CGFloat k = MIN(22.0 / 367, h / 410);
	NSPoint center = NSMakePoint(x + 11, (h - 410*k)/2 + 226.5*k);
	BOOL known = [c[@"known"] boolValue], failed = [c[@"failed"] boolValue];
	NSColor *ink = failed ? NSColor.systemRedColor : [c[@"low"] boolValue] ? NSColor.systemOrangeColor : label;
	NSColor *muted = [label colorWithAlphaComponent:0.22];
	mpRingArc(center, 155.975*k, 55.05*k, 1, muted);
	if (known) mpRingArc(center, 155.975*k, 55.05*k, [c[@"share"] doubleValue], ink);
	mpRingWaves(c, center, k, label);
	NSDictionary *attrs = @{NSFontAttributeName: [NSFont systemFontOfSize:130*k weight:NSFontWeightBold], NSForegroundColorAttributeName: ink};
	NSString *letter = c[@"letter"];
	[letter drawAtPoint:NSMakePoint(x + 11 - [letter sizeWithAttributes:attrs].width/2, (h-410*k)/2 - 1*k) withAttributes:attrs];
	if (failed) {
		[ink setFill];
		[[NSBezierPath bezierPathWithRoundedRect:NSMakeRect(center.x-95*k, center.y-26*k, 190*k, 52*k) xRadius:26*k yRadius:26*k] fill];
		return;
	}
	if (!known) {
		NSDictionary *a = @{NSFontAttributeName: [NSFont systemFontOfSize:6 weight:NSFontWeightMedium], NSForegroundColorAttributeName: label};
		[@"?" drawAtPoint:NSMakePoint(center.x-2, center.y-4) withAttributes:a];
		return;
	}
	[ink setFill];
	[[NSBezierPath bezierPathWithOvalInRect:NSMakeRect(center.x-19*k, center.y-19*k, 38*k, 38*k)] fill];
	if (![c[@"paced"] boolValue]) return;
	CGFloat pace = [c[@"pace"] doubleValue], fill = MIN(1, fabs(pace)/0.25);
	for (int i = 0; i < 2; i++) {
		CGFloat inner = (i == 0 ? 42 : 82)*k, outer = (i == 0 ? 69 : 109)*k;
		for (int side = -1; side <= 1; side += 2) {
			CGFloat middle = side * 90;
			mpRingFan(center, inner, outer, middle, 1, muted);
			if ((pace > 0 && side == -1) || (pace < 0 && side == 1))
				mpRingFan(center, inner, outer, middle, MAX(0, fill*2-i), ink);
		}
	}
}

// mpDraw draws the bird, then each cell, top down (a flipped context).
static void mpDraw(NSArray *cells, NSImage *bird, CGFloat h) {
	if (bird != nil) mpTinted(bird, NSMakeRect(0, 0, h, h));
	CGFloat x = mpIn(bird, h);
	NSColor *ink = [NSColor labelColor];
	BOOL first = YES;
	for (NSDictionary *c in cells) {
		if ([c[@"ring"] boolValue]) {
			x += mpLead(c, first);
			mpRing(c, x, h, ink);
			x += mpColumn(c);
			first = NO;
			continue;
		}
		if ([c[@"plain"] boolValue]) {
			if (!first) {
				[[ink colorWithAlphaComponent:0.35] set];
				NSRectFillUsingOperation(NSMakeRect(x + mpSepGap, round((h - mpSepHigh) / 2), mpSep, mpSepHigh), NSCompositingOperationSourceOver);
			}
			x += mpLead(c, first);
			first = NO;
			mpRows(c, x, h, ink);
			x += mpColumn(c);
			continue;
		}
		x += mpLead(c, first) - mpLogo - mpLogoGap;
		first = NO;
		NSRect logo = NSMakeRect(x, round((h - mpLogo) / 2), mpLogo, mpLogo);
		NSImage *icon = c[@"icon"];
		if (icon != nil) {
			mpMasked(icon, logo);
		} else {
			// no logo: the name's first letter in a ring
			NSBezierPath *ring = [NSBezierPath bezierPathWithRoundedRect:NSInsetRect(logo, 0.75, 0.75) xRadius:3.5 yRadius:3.5];
			ring.lineWidth = 1.2;
			[ink set];
			[ring stroke];
			NSDictionary *a = @{NSFontAttributeName: [NSFont systemFontOfSize:8.5 weight:NSFontWeightBold], NSForegroundColorAttributeName: ink};
			NSString *l = c[@"letter"];
			NSSize s = [l sizeWithAttributes:a];
			[l drawAtPoint:NSMakePoint(NSMidX(logo) - s.width / 2, NSMidY(logo) - s.height / 2) withAttributes:a];
		}
		x += mpLogo + mpLogoGap;
		mpRows(c, x, h, ink);
		x += mpColumn(c);
	}
}

// mpImage is the bird and the cells as one image, drawn afresh each time it
// is shown so its colours follow the menu bar's, and sharp at any scale.
static NSImage *mpImage(NSArray *cells, NSImage *bird, CGFloat h) {
	NSImage *im = [NSImage imageWithSize:NSMakeSize(mpWidth(cells, h, mpIn(bird, h)), h) flipped:YES drawingHandler:^BOOL(NSRect r) {
		mpDraw(cells, bird, h);
		return YES;
	}];
	im.cacheMode = NSImageCacheNever;
	return im;
}

// mpStarts writes each cell's start in image points, following mpDraw's layout.
// Plain cells start at their separator; others start at their logo.
static void mpStarts(NSArray *cells, CGFloat in, CGFloat *out) {
	CGFloat pos = in;
	BOOL first = YES;
	int i = 0;
	for (NSDictionary *c in cells) {
		if ([c[@"ring"] boolValue]) {
			pos += mpLead(c, first);
			out[i++] = pos;
			pos += mpColumn(c);
			first = NO;
			continue;
		}
		if ([c[@"plain"] boolValue]) {
			out[i] = first ? pos : pos + mpSepGap;
			pos += mpLead(c, first);
		} else {
			pos += mpLead(c, first) - mpLogo - mpLogoGap;
			out[i] = pos;
			pos += mpLogo + mpLogoGap;
		}
		pos += mpColumn(c);
		i++;
		first = NO;
	}
}

// mpCellAt returns the cell at x, including its trailing gap.
// The bird and points outside the image return -1.
static int mpCellAt(NSArray *cells, CGFloat in, CGFloat x, CGFloat w) {
	if (cells.count == 0 || x < in || x >= w) return -1;
	int n = (int)cells.count;
	CGFloat starts[n];
	mpStarts(cells, in, starts);
	for (int i = 0; i < n; i++) {
		if (x < (i + 1 < n ? starts[i + 1] : w)) return i;
	}
	return -1;
}

static NSStatusBarButton *mpFind(NSView *v) {
	if ([v isKindOfClass:[NSStatusBarButton class]]) return (NSStatusBarButton *)v;
	for (NSView *s in v.subviews) {
		NSStatusBarButton *b = mpFind(s);
		if (b != nil) return b;
	}
	return nil;
}

// mpButton is magpie's one status item's button, in the status bar's window.
static NSStatusBarButton *mpButton(void) {
	for (NSWindow *w in NSApp.windows) {
		NSStatusBarButton *b = mpFind(w.contentView);
		if (b != nil) return b;
	}
	return nil;
}

// What the item shows while the cells are up; the bird changes as it flaps.
static NSArray *mpShown;
static NSImage *mpBird;

// mpCellClicked copies the drawn card's identity to Go before it can be replaced.
extern void mpCellClicked(char *id);

static CGFloat mpHitW = 0;

// Used by the single status-item monitor and native tests. Modified clicks
// retain the button's normal action; an unmodified quota click uses its ID.
// Shown without the bird, the cards are the whole item, and a click on them
// is the item's own: it opens the panel on the tab left last, as a click on
// the bird does (the owner: with the bird off, every click opened Allowances,
// since there was nowhere left to click that didn't).
static BOOL mpClickCell(NSArray *cells, BOOL bird, CGFloat in, CGFloat x, CGFloat w, NSEventModifierFlags flags) {
	if (!bird) return NO;
	if (flags & (NSEventModifierFlagCommand | NSEventModifierFlagControl | NSEventModifierFlagOption | NSEventModifierFlagShift)) return NO;
	int i = mpCellAt(cells, in, x, w);
	if (i < 0) return NO;
	mpCellClicked((char *)[cells[i][@"id"] UTF8String]);
	return YES;
}

static void mpApply(void) {
	NSStatusBarButton *b = mpButton();
	if (b == nil || mpShown == nil) return;
	b.title = @"";
	NSImage *im = mpImage(mpShown, mpBird, [[NSStatusBar systemStatusBar] thickness]);
	b.image = im;
	b.imagePosition = NSImageOnly;
	mpHitW = im.size.width;
}

// Stop the animation timer as soon as no visible ring is active. Idle and
// reduced-motion modes never run a frame timer, nor trigger quota reads.
static void mpMotionUpdate(void) {
	BOOL active = NO;
	for (NSDictionary *c in mpShown) if ([c[@"ring"] boolValue] && mpActiveCount(c)>0) { active=YES; break; }
	BOOL reduced = mpReduced || NSWorkspace.sharedWorkspace.accessibilityDisplayShouldReduceMotion;
	if (!active || reduced || mpButton()==nil) {
		[mpMotion invalidate]; [mpMotion release]; mpMotion=nil;
	} else if (mpMotion==nil) {
		mpMotion = [[NSTimer timerWithTimeInterval:1.0/12 repeats:YES block:^(NSTimer *timer) {
			if (mpButton()==nil || NSWorkspace.sharedWorkspace.accessibilityDisplayShouldReduceMotion) { mpMotionUpdate(); return; }
			mpApply();
		}] retain];
		[NSRunLoop.mainRunLoop addTimer:mpMotion forMode:NSRunLoopCommonModes];
	}
}

static void mpActivityTip(void) {
	NSMutableArray *parts=[NSMutableArray array];
	if (mpBaseTip.length) [parts addObject:mpBaseTip];
	for (NSDictionary *c in mpShown) {
		NSDictionary *a=mpActivities[[c[@"id"] lowercaseString]];
		if ([c[@"ring"] boolValue] && a!=nil) [parts addObject:[NSString stringWithFormat:@"%@\n%@",c[@"title"],a[@"tip"]]];
	}
    if (mpActivities[@""]!=nil) {
        for (NSDictionary *c in mpShown) if ([c[@"ring"] boolValue]) { [parts addObject:mpActivities[@""][@"tip"]]; break; }
    }
	NSString *tip=[parts componentsJoinedByString:@"\n\n"];
	mpButton().toolTip=tip.length ? tip : nil;
}

static void mpActivity(mpTrayCell *cells,int n,int reduced) {
	@autoreleasepool {
		NSMutableDictionary *all=[[NSMutableDictionary alloc] init];
		for (int i=0;i<n;i++) {
			NSString *id=[NSString stringWithUTF8String:cells[i].id];
			all[id.lowercaseString]=@{@"count":@(cells[i].active),@"tip":[NSString stringWithUTF8String:cells[i].rows]};
		}
		dispatch_sync(dispatch_get_main_queue(), ^{
			[mpActivities release]; mpActivities=all; mpReduced=reduced!=0;
			mpMotionUpdate(); mpApply(); mpActivityTip();
		});
	}
}

static NSImage *mpBirdImage(const void *bird, int len) {
	if (bird == NULL || len <= 0) return nil; // the cells alone
	NSImage *im = [[NSImage alloc] initWithData:[NSData dataWithBytes:bird length:len]];
	return im;
}

// mpShow puts the cells up beside the bird; 0 when there is no item to put
// them on. On the main queue after what Wails has queued there (the label
// taken off), so nothing of Wails' lands after it.
// It runs on a Go thread, which has no autorelease pool of its own to
// drain what reading the cells leaves (every few minutes, for good).
static int mpShow(mpTrayCell *cells, int n, const void *bird, int len) {
	__block int ok = 0;
	@autoreleasepool {
		NSArray *cs = [mpCells(cells, n) retain];
		NSImage *bi = mpBirdImage(bird, len);
		dispatch_sync(dispatch_get_main_queue(), ^{
			if (mpButton() == nil) return;
			[mpShown release];
			[mpBird release];
			mpShown = cs;
            static dispatch_once_t motionObserver;
            dispatch_once(&motionObserver, ^{
                [NSWorkspace.sharedWorkspace.notificationCenter addObserverForName:NSWorkspaceAccessibilityDisplayOptionsDidChangeNotification object:nil queue:NSOperationQueue.mainQueue usingBlock:^(NSNotification *note){mpMotionUpdate();mpApply();}];
            });
			mpBird = [bi retain];
			mpMotionUpdate(); mpActivityTip();
			mpApply();
			ok = 1;
		});
		if (!ok) [cs release];
		[bi release];
	}
	return ok;
}

// mpFrame is a frame of the bird's flap while the cells are up; 0 when
// they aren't, and the frame is Wails' to set as the icon.
static int mpFrame(const void *bird, int len) {
	__block int ok = 0;
	@autoreleasepool {
		NSImage *bi = mpBirdImage(bird, len);
		dispatch_sync(dispatch_get_main_queue(), ^{
			if (mpShown == nil) return;
			ok = 1;
			if (mpBird == nil) return; // shown without the bird: no flap
			[mpBird release];
			mpBird = [bi retain];
			mpApply();
		});
		[bi release];
	}
	return ok;
}

// mpTipOn is the button's tooltip, none for "": the text as given, its
// lines kept, as AppKit shows a tooltip's.
static void mpTipOn(NSStatusBarButton *b, NSString *s) {
	b.toolTip = s.length ? s : nil;
}

// mpTip puts the tooltip on magpie's status item; 0 while the item isn't
// up yet. Wails (v3.0.0-beta.24) leaves SetTooltip a no-op on macOS
// ("Tooltips not supported on macOS").
static int mpTip(const char *tip) {
	__block int ok = 0;
	@autoreleasepool {
		NSString *s = [NSString stringWithUTF8String:tip];
		dispatch_sync(dispatch_get_main_queue(), ^{
			NSStatusBarButton *b = mpButton();
			if (b == nil) return;
			[mpBaseTip release]; mpBaseTip=[s copy];
			mpActivityTip();
			ok = 1;
		});
	}
	return ok;
}

// mpTipShown is the tooltip a status bar button given tip shows, for
// tests, which have no status item nor main queue to ask.
static char *mpTipShown(const char *tip) {
	@autoreleasepool {
		NSStatusBarButton *b = [[[NSStatusBarButton alloc] initWithFrame:NSMakeRect(0, 0, 22, 22)] autorelease];
		mpTipOn(b, [NSString stringWithUTF8String:tip]);
		return strdup(b.toolTip ? b.toolTip.UTF8String : "");
	}
}

// mpHide forgets the cells; the icon Wails sets next puts the bird back.
static void mpHide(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[mpShown release];
		mpShown = nil;
		mpMotionUpdate();
		mpHitW = 0; // every click is the item's whole one again
	});
}

// mpHighlight lights the item while its panel is open, as the system's own
// items do.
static void mpHighlight(int on) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[mpButton() highlight:on != 0];
	});
}

// mpOwnClicks hands a left click on the item to its action before the
// button sees it: the button's own tracking would light it on mouse-down
// and put it out on mouse-up, a flicker before mpHighlight's. One with
// Command held is left to the button, which is what lets the system move
// the item (#792: Command-dragging it, as Hidden Bar asks, did nothing).
static void mpOwnClicks(void) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		dispatch_async(dispatch_get_main_queue(), ^{
			[NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDown handler:^NSEvent *(NSEvent *e) {
				NSStatusBarButton *b = mpButton();
				if (b == nil || e.window != b.window) return e;
				NSPoint p = [b convertPoint:e.locationInWindow fromView:nil];
				if (!NSPointInRect(p, b.bounds)) return e;
				if (e.modifierFlags & NSEventModifierFlagCommand) return e;
				// Route quota clicks before the generic action, within this
				// one monitor; local monitor ordering is not guaranteed.
				CGFloat x = p.x - (b.bounds.size.width - mpHitW) / 2;
				if (mpClickCell(mpShown, mpBird != nil, mpIn(mpBird, [[NSStatusBar systemStatusBar] thickness]), x, mpHitW, e.modifierFlags)) return nil;
				[NSApp sendAction:b.action to:b.target from:b];
				return nil;
			}];
		});
	});
}

// mpPNG draws the image as the menu bar would at scale, light or dark, on
// the bar's colour when bg is set: for tests and previews.
static void *mpPNG(mpTrayCell *cells, int n, const void *bird, int len, CGFloat h, CGFloat scale, int dark, int bg, double phase, int reduced, int *outLen, int *pxW, int *pxH) {
	__block void *out = NULL;
	@autoreleasepool {
		NSArray *cs = mpCells(cells, n);
		NSImage *bi = [mpBirdImage(bird, len) autorelease];
		CGFloat w = mpWidth(cs, h, mpIn(bi, h));
		NSBitmapImageRep *rep = [[[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:(NSInteger)ceil(w * scale) pixelsHigh:(NSInteger)ceil(h * scale)
			bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0] autorelease];
		rep.size = NSMakeSize(w, h);
		*pxW = (int)rep.pixelsWide;
		*pxH = (int)rep.pixelsHigh;
		NSGraphicsContext *g = [NSGraphicsContext graphicsContextWithBitmapImageRep:rep];
		NSAppearance *look = [NSAppearance appearanceNamed:dark ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua];
		[NSGraphicsContext saveGraphicsState];
		[NSGraphicsContext setCurrentContext:g];
		double savedPhase=mpPreviewPhase; mpPreviewPhase=phase; BOOL savedReduced=mpPreviewReduced; mpPreviewReduced=reduced!=0;
		void (^paint)(void) = ^{
			if (bg) {
				[(dark ? [NSColor colorWithSRGBRed:0.16 green:0.16 blue:0.17 alpha:1] : [NSColor colorWithSRGBRed:0.93 green:0.93 blue:0.94 alpha:1]) set];
				NSRectFill(NSMakeRect(0, 0, w, h));
			}
			[mpImage(cs, bi, h) drawInRect:NSMakeRect(0, 0, w, h)];
		};
		if (@available(macOS 11.0, *)) {
			[look performAsCurrentDrawingAppearance:paint];
		} else {
			paint(); // the system's own look, light or dark as it is
		}
		mpPreviewPhase=savedPhase; mpPreviewReduced=savedReduced;
		[g flushGraphics];
		[NSGraphicsContext restoreGraphicsState];
		NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		*outLen = (int)png.length;
		out = malloc(png.length);
		memcpy(out, png.bytes, png.length);
	}
	return out;
}

// Native hit-testing helpers for Go tests, using the actual image layout.
static int mpCellAtCells(mpTrayCell *cells, int n, CGFloat h, CGFloat x) {
	@autoreleasepool {
		NSArray *cs = mpCells(cells, n);
		return mpCellAt(cs, h + mpBirdGap, x, mpWidth(cs, h, h + mpBirdGap));
	}
}

static CGFloat mpCellStartAt(mpTrayCell *cells, int n, CGFloat h, int i) {
	@autoreleasepool {
		NSArray *cs = mpCells(cells, n);
		if (i < 0 || i >= n) return -1;
		CGFloat starts[n];
		mpStarts(cs, h + mpBirdGap, starts);
		return starts[i];
	}
}

// Exercise the same identity lookup and C-to-Go callback without a live status item.
static int mpClickCellsAt(mpTrayCell *cells, int n, int bird, CGFloat h, CGFloat x, unsigned long flags) {
	@autoreleasepool {
		NSArray *cs = mpCells(cells, n);
		CGFloat in = bird ? h + mpBirdGap : 0;
		return mpClickCell(cs, bird, in, x, mpWidth(cs, h, in), flags);
	}
}

*/
import "C"

import (
	"strings"
	"unsafe"
)

// The Mac's menu bar draws the cards as one image beside the bird (see
// trayusage.go): drawn by AppKit, as the bar's own text is, in its colour
// light or dark and sharp at any scale. Wails sizes an icon it is given to
// a square, so the image is set on the status item's button here; the
// bird's flap goes through it while it is up (trayImageFrame).

// withCells hands the cells to C for the length of f.
func withCells(cells []trayCell, f func(*C.mpTrayCell, C.int)) {
	if len(cells) == 0 {
		f(nil, 0)
		return
	}
	cs := (*[1 << 16]C.mpTrayCell)(C.malloc(C.size_t(len(cells)) * C.size_t(unsafe.Sizeof(C.mpTrayCell{}))))[:len(cells):len(cells)]
	var owned []unsafe.Pointer
	for i, c := range cells {
		cs[i] = C.mpTrayCell{id: C.CString(c.ID), title: C.CString(c.Title), letter: C.CString(c.Letter), rows: C.CString(strings.Join(c.Rows, "\n"))}
		owned = append(owned, unsafe.Pointer(cs[i].id), unsafe.Pointer(cs[i].title), unsafe.Pointer(cs[i].letter), unsafe.Pointer(cs[i].rows))
		if len(c.Icon) > 0 {
			cs[i].icon, cs[i].iconLen = C.CBytes(c.Icon), C.int(len(c.Icon))
			owned = append(owned, cs[i].icon)
		}
		if r := c.Ring; r != nil {
			cs[i].ring = 1
			cs[i].active = C.int(r.Active)
			if r.Share != nil {
				cs[i].known = 1
				cs[i].share = C.double(*r.Share)
			}
			if r.Pace != nil {
				cs[i].paced = 1
				cs[i].pace = C.double(*r.Pace)
			}
			if r.Error {
				cs[i].failed = 1
			}
			if r.Low {
				cs[i].low = 1
			}
		}
		if c.Mono {
			cs[i].mono = 1
		}
		if c.Plain {
			cs[i].plain = 1
		}
	}
	defer func() {
		for _, p := range owned {
			C.free(p)
		}
		C.free(unsafe.Pointer(&cs[0]))
	}()
	f(&cs[0], C.int(len(cells)))
}

func cBytes(b []byte) (unsafe.Pointer, C.int) {
	if len(b) == 0 {
		return nil, 0
	}
	return C.CBytes(b), C.int(len(b))
}

// trayImageShow puts the cells up beside the bird, or alone when bird is
// nil; false when the item can't be found, and the text is to be shown
// instead.
func trayImageShow(cells []trayCell, bird []byte) bool {
	ok := false
	b, n := cBytes(bird)
	defer C.free(b)
	withCells(cells, func(cs *C.mpTrayCell, count C.int) { ok = C.mpShow(cs, count, b, n) != 0 })
	return ok
}

// trayImageFrame sets a frame of the bird's flap while the cells are up;
// false when they aren't, for Wails to set it as the icon.
func trayImageFrame(bird []byte) bool {
	b, n := cBytes(bird)
	defer C.free(b)
	return C.mpFrame(b, n) != 0
}

// trayTipNative puts the tooltip on the menu bar item, where Wails' own
// SetTooltip does nothing; false while the item isn't up yet, to be tried
// again. A var for tests, which have no main queue for it to wait on.
var trayTipNative = func(tip string) bool {
	cs := C.CString(tip)
	defer C.free(unsafe.Pointer(cs))
	return C.mpTip(cs) != 0
}

// trayTipShown is the tooltip a menu bar button given tip shows.
func trayTipShown(tip string) string {
	cs := C.CString(tip)
	defer C.free(unsafe.Pointer(cs))
	out := C.mpTipShown(cs)
	defer C.free(unsafe.Pointer(out))
	return C.GoString(out)
}

// trayImageHide takes the cells down; the icon set next is the bird alone.
func trayImageHide() { C.mpHide() }

// trayHighlight lights the tray icon, or puts it out.
func trayHighlight(on bool) { C.mpHighlight(cBool(on)) }

// trayOwnClicks keeps the button's own highlight out of a click (mpOwnClicks).
func trayOwnClicks() { C.mpOwnClicks() }

// trayImagePNG is the image as the menu bar h points high draws it at
// scale (2 for a Retina screen), light or dark, on the bar's colour when
// bg; with its size in pixels.
func trayImagePNG(cells []trayCell, bird []byte, h, scale float64, dark, bg bool) (png []byte, w, ht int) {
	return trayImagePNGPhase(cells, bird, h, scale, dark, bg, 0)
}

func trayImagePNGPhase(cells []trayCell, bird []byte, h, scale float64, dark, bg bool, phase float64) (png []byte, w, ht int) {
	return trayImagePNGRender(cells, bird, h, scale, dark, bg, phase, false)
}

func trayImagePNGRender(cells []trayCell, bird []byte, h, scale float64, dark, bg bool, phase float64, reduced bool) (png []byte, w, ht int) {
	b, n := cBytes(bird)
	defer C.free(b)
	withCells(cells, func(cs *C.mpTrayCell, count C.int) {
		var size, pw, ph C.int
		p := C.mpPNG(cs, count, b, n, C.CGFloat(h), C.CGFloat(scale), cBool(dark), cBool(bg), C.double(phase), cBool(reduced), &size, &pw, &ph)
		if p != nil {
			png = C.GoBytes(p, size)
			C.free(p)
		}
		w, ht = int(pw), int(ph)
	})
	return png, w, ht
}

// trayRowFont is the font mpRowFont draws n rows in: its size, its weight
// trait (0 regular, about 0.23 medium, 0.3 semibold), whether its digits
// are of one width, and the menu bar font's size.
func trayRowFont(n int) (size, weight float64, mono bool, menuBar float64) {
	var s, w, m C.CGFloat
	var mo C.int
	C.mpRowFontInfo(C.int(n), &s, &w, &mo, &m)
	return float64(s), float64(w), mo != 0, float64(m)
}

func cBool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// These native test wrappers stay here because Go does not support cgo
// imports in _test.go files. They do not install monitors or create windows.
// trayImageCellAt tests the native hit map; -1 means no quota cell.
func trayImageCellAt(cells []trayCell, h, x float64) int {
	at := -1
	withCells(cells, func(cs *C.mpTrayCell, count C.int) {
		at = int(C.mpCellAtCells(cs, count, C.CGFloat(h), C.CGFloat(x)))
	})
	return at
}

// trayImageCellStart returns cell i's start in image points, or -1 if absent.
func trayImageCellStart(cells []trayCell, h float64, i int) float64 {
	s := -1.0
	withCells(cells, func(cs *C.mpTrayCell, count C.int) {
		s = float64(C.mpCellStartAt(cs, count, C.CGFloat(h), C.int(i)))
	})
	return s
}

func trayImageClickAt(cells []trayCell, bird bool, h, x float64, modifiers uint64) bool {
	hit := false
	b := C.int(0)
	if bird {
		b = 1
	}
	withCells(cells, func(cs *C.mpTrayCell, count C.int) {
		hit = C.mpClickCellsAt(cs, count, b, C.CGFloat(h), C.CGFloat(x), C.ulong(modifiers)) != 0
	})
	return hit
}

// trayImageActivity updates only the motion and its exact model counts.
func trayImageActivity(cells []trayCell, reduced bool) {
	withCells(cells, func(cs *C.mpTrayCell, n C.int) { C.mpActivity(cs, n, cBool(reduced)) })
}
