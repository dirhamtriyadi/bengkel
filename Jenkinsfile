pipeline {
  agent any

  options {
    timestamps()
    disableConcurrentBuilds()
    buildDiscarder(logRotator(numToKeepStr: '20'))
  }

  environment {
    REGISTRY = credentials('bengkel-container-registry')
    REGISTRY_HOST = 'registry.example.com'
    API_IMAGE = "${REGISTRY_HOST}/bengkel/api"
    WEB_IMAGE = "${REGISTRY_HOST}/bengkel/web"
    IMAGE_TAG = "${BUILD_NUMBER}-${GIT_COMMIT.take(8)}"
  }

  stages {
    stage('Checkout') {
      steps { checkout scm }
    }
    stage('Backend quality') {
      agent { docker { image 'golang:1.24-alpine'; reuseNode true } }
      steps {
        sh 'go mod download'
        sh 'go vet ./...'
        sh 'go test -race -coverprofile=coverage.out ./...'
        sh 'go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g ./cmd/api/main.go -o ./docs --parseDependency --parseInternal'
        sh 'git diff --exit-code -- docs'
      }
    }
    stage('Frontend quality') {
      agent { docker { image 'node:24-alpine'; reuseNode true } }
      steps {
        dir('frontend') {
          sh 'npm ci'
          sh 'npm run lint'
          sh 'npm run typecheck'
          sh 'npm run build'
        }
      }
    }
    stage('Build and push images') {
      when { branch 'main' }
      steps {
        sh 'echo "$REGISTRY_PSW" | docker login "$REGISTRY_HOST" -u "$REGISTRY_USR" --password-stdin'
        sh 'docker build -t "$API_IMAGE:$IMAGE_TAG" -t "$API_IMAGE:latest" .'
        sh 'docker build -t "$WEB_IMAGE:$IMAGE_TAG" -t "$WEB_IMAGE:latest" frontend'
        sh 'docker push "$API_IMAGE:$IMAGE_TAG"'
        sh 'docker push "$WEB_IMAGE:$IMAGE_TAG"'
        sh 'docker push "$API_IMAGE:latest"'
        sh 'docker push "$WEB_IMAGE:latest"'
      }
    }
    stage('Deploy production') {
      when { branch 'main' }
      steps {
        input message: 'Deploy build ke production?', ok: 'Deploy'
        sshagent(credentials: ['bengkel-production-ssh']) {
          sh '''
            ssh -o StrictHostKeyChecking=yes "$DEPLOY_USER@$DEPLOY_HOST" \
              "cd /opt/bengkel && API_IMAGE='$API_IMAGE:$IMAGE_TAG' WEB_IMAGE='$WEB_IMAGE:$IMAGE_TAG' ./deploy.sh"
          '''
        }
      }
    }
  }

  post {
    always {
      junit allowEmptyResults: true, testResults: '**/junit*.xml'
      archiveArtifacts allowEmptyArchive: true, artifacts: 'coverage.out,docs/swagger.*'
      cleanWs()
    }
  }
}
