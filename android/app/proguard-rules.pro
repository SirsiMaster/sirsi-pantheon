# Pantheon ProGuard Rules

# Keep gomobile generated bindings
-keep class mobile.** { *; }
-keep class go.** { *; }

# Keep kotlinx.serialization
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.AnnotationsKt

-keepclassmembers class kotlinx.serialization.json.** {
    *** Companion;
}
-keepclasseswithmembers class kotlinx.serialization.json.** {
    kotlinx.serialization.KSerializer serializer(...);
}

# Keep Pantheon models for serialization
-keep,includedescriptorclasses class ai.sirsi.pantheon.models.**$$serializer { *; }
-keepclassmembers class ai.sirsi.pantheon.models.** {
    *** Companion;
}
-keepclasseswithmembers class ai.sirsi.pantheon.models.** {
    kotlinx.serialization.KSerializer serializer(...);
}
