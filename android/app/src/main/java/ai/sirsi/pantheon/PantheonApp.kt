package ai.sirsi.pantheon

import android.app.Application

/**
 * Pantheon Application class.
 * Initializes the Go mobile bridge on app startup.
 */
class PantheonApp : Application() {

    override fun onCreate() {
        super.onCreate()
        instance = this
    }

    companion object {
        lateinit var instance: PantheonApp
            private set
    }
}
