<?php

namespace FlowCatalyst\Generated\Endpoint;

class EnableClientApplication extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $applicationId;
    protected $id;
    /**
     * @param string $applicationId
     * @param string $id
     */
    public function __construct(string $applicationId, string $id)
    {
        $this->applicationId = $applicationId;
        $this->id = $id;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{applicationId}', '{id}'], [$this->applicationId, $this->id], '/api/admin/clients/{id}/applications/{applicationId}/enable');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\EnableClientApplicationBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\EnableClientApplicationNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\EnableClientApplicationUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\EnableClientApplicationForbiddenException
     *
     * @return null
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return json_decode($body);
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\EnableClientApplicationBadRequestException($response);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\EnableClientApplicationNotFoundException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\EnableClientApplicationUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\EnableClientApplicationForbiddenException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}